package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/fleetdm/fleet/server/kolide"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

var userSearchColumns = []string{"name", "email"}

// NewUser creates a new user
func (d *Datastore) NewUser(user *kolide.User) (*kolide.User, error) {
	sqlStatement := `
      INSERT INTO users (
      	password,
      	salt,
      	name,
      	username,
      	email,
      	admin,
      	enabled,
      	admin_forced_password_reset,
      	gravatar_url,
      	position,
        sso_enabled,
		global_role
      ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
      `
	result, err := d.db.Exec(sqlStatement, user.Password, user.Salt, user.Name,
		user.Username, user.Email, user.Admin, user.Enabled,
		user.AdminForcedPasswordReset, user.GravatarURL, user.Position, user.SSOEnabled,
		user.GlobalRole)
	if err != nil {
		return nil, errors.Wrap(err, "create new user")
	}

	id, _ := result.LastInsertId()
	user.ID = uint(id)

	if err := d.saveTeamsForUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (d *Datastore) findUser(searchCol string, searchVal interface{}) (*kolide.User, error) {
	sqlStatement := fmt.Sprintf(
		"SELECT * FROM users "+
			"WHERE %s = ? LIMIT 1",
		searchCol,
	)

	user := &kolide.User{}

	err := d.db.Get(user, sqlStatement, searchVal)
	if err != nil && err == sql.ErrNoRows {
		return nil, notFound("User").
			WithMessage(fmt.Sprintf("with %s=%v", searchCol, searchVal))
	} else if err != nil {
		return nil, errors.Wrap(err, "find user")
	}

	if err := d.loadTeamsForUsers([]*kolide.User{user}); err != nil {
		return nil, errors.Wrap(err, "load teams")
	}

	return user, nil
}

// User retrieves a user by name
func (d *Datastore) User(username string) (*kolide.User, error) {
	return d.findUser("username", username)
}

// ListUsers lists all users with limit, sort and offset passed in with
// kolide.ListOptions
func (d *Datastore) ListUsers(opt kolide.ListOptions) ([]*kolide.User, error) {
	sqlStatement := `
		SELECT * FROM users
		WHERE TRUE
	`

	sqlStatement, params := searchLike(sqlStatement, nil, opt.MatchQuery, userSearchColumns...)
	sqlStatement = appendListOptionsToSQL(sqlStatement, opt)
	users := []*kolide.User{}

	if err := d.db.Select(&users, sqlStatement, params...); err != nil {
		return nil, errors.Wrap(err, "list users")
	}

	if err := d.loadTeamsForUsers(users); err != nil {
		return nil, errors.Wrap(err, "load teams")
	}

	return users, nil

}

func (d *Datastore) UserByEmail(email string) (*kolide.User, error) {
	return d.findUser("email", email)
}

func (d *Datastore) UserByID(id uint) (*kolide.User, error) {
	return d.findUser("id", id)
}

func (d *Datastore) SaveUser(user *kolide.User) error {
	sqlStatement := `
      UPDATE users SET
      	username = ?,
      	password = ?,
      	salt = ?,
      	name = ?,
      	email = ?,
      	admin = ?,
      	enabled = ?,
      	admin_forced_password_reset = ?,
      	gravatar_url = ?,
      	position = ?,
        sso_enabled = ?,
		global_role = ?
      WHERE id = ?
      `
	result, err := d.db.Exec(sqlStatement, user.Username, user.Password,
		user.Salt, user.Name, user.Email, user.Admin, user.Enabled,
		user.AdminForcedPasswordReset, user.GravatarURL, user.Position, user.SSOEnabled,
		user.GlobalRole, user.ID)
	if err != nil {
		return errors.Wrap(err, "save user")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "rows affected save user")
	}
	if rows == 0 {
		return notFound("User").WithID(user.ID)
	}

	// REVIEW: Check if teams have been set?
	if err := d.saveTeamsForUser(user); err != nil {
		return err
	}

	return nil
}

// loadTeamsForUsers will load the teams/roles for the provided users.
func (d *Datastore) loadTeamsForUsers(users []*kolide.User) error {
	userIDs := make([]uint, 0, len(users)+1)
	// Make sure the slice is never empty for IN by filling a nonexistent ID
	userIDs = append(userIDs, 0)
	idToUser := make(map[uint]*kolide.User, len(users))
	for _, u := range users {
		// Initialize empty slice so we get an array in JSON responses instead
		// of null if it is empty
		u.Teams = []kolide.UserTeam{}
		// Track IDs for queries and matching
		userIDs = append(userIDs, u.ID)
		idToUser[u.ID] = u
	}

	sql := `
		SELECT ut.team_id AS id, ut.user_id, ut.role, t.name
		FROM user_teams ut INNER JOIN teams t ON ut.team_id = t.id
		WHERE ut.user_id IN (?)
		ORDER BY user_id, team_id
	`
	sql, args, err := sqlx.In(sql, userIDs)
	if err != nil {
		return errors.Wrap(err, "sqlx.In loadTeamsForUsers")
	}

	var rows []struct {
		kolide.UserTeam
		UserID uint `db:"user_id"`
	}
	if err := d.db.Select(&rows, sql, args...); err != nil {
		return errors.Wrap(err, "get loadTeamsForUsers")
	}

	// Map each row to the appropriate user
	for _, r := range rows {
		user := idToUser[r.UserID]
		user.Teams = append(user.Teams, r.UserTeam)
	}

	return nil
}

func (d *Datastore) saveTeamsForUser(user *kolide.User) error {
	// Do a full teams update by deleting existing teams and then inserting all
	// the current teams in a single transaction.
	if err := d.withRetryTxx(func(tx *sqlx.Tx) error {
		// Delete before insert
		sql := `DELETE FROM user_teams WHERE user_id = ?`
		if _, err := tx.Exec(sql, user.ID); err != nil {
			return errors.Wrap(err, "delete existing teams")
		}

		if len(user.Teams) == 0 {
			return nil
		}

		// Bulk insert
		const valueStr = "(?,?,?),"
		var args []interface{}
		for _, userTeam := range user.Teams {
			args = append(args, user.ID, userTeam.Team.ID, userTeam.Role)
		}
		sql = "INSERT INTO user_teams (user_id, team_id, role) VALUES " +
			strings.Repeat(valueStr, len(user.Teams))
		sql = strings.TrimSuffix(sql, ",")
		if _, err := tx.Exec(sql, args...); err != nil {
			return errors.Wrap(err, "insert teams")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "save teams for user")
	}
	return nil
}
