# Reference Architectures

You can easily run Fleet on a single VPS that would be capable of supporting hundreds if not thousands of hosts, but
this page details an [opinionated view](https://github.com/fleetdm/fleet/tree/main/tools/terraform) of running Fleet in a production environment, as
well as different configuration strategies to enable High Availability (HA).

## Availability Components

There are a few strategies that can be used to ensure high availability:
- Database HA
- Traffic load balancing

### Database HA

Fleet recommends RDS Aurora MySQL when running on AWS. More details about backups/snapshots can be found
[here](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Aurora.Managing.Backups.html). It is also
possible to dynamically scale read replicas to increase performance and [enable database fail-over](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Concepts.AuroraHighAvailability.html).
It is also possible to use [Aurora Global](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-global-database.html) to
span multiple regions for more advanced configurations(_not included in the [reference terraform](https://github.com/fleetdm/fleet/tree/main/tools/terraform)_).

In some cases adding a read replica can increase database performance for specific access patterns. In scenarios when automating the API or with `fleetctl`
there can be benefits to read performance.

### Traffic load balancing
Load balancing enables distributing request traffic over many instances of the backend application. Using AWS Application
Load Balancer can also [offload SSL termination](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/create-https-listener.html), freeing Fleet to spend the majority of it's allocated compute dedicated 
to its core functionality. More details about ALB can be found [here](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html).

_**Note if using [terraform reference architecture](https://github.com/fleetdm/fleet/tree/main/tools/terraform#terraform) all configurations can dynamically scale based on load(cpu/memory) and all configurations
assume On-Demand pricing (savings are available through Reserved Instances). Calculations do not take into account NAT gateway charges or other networking related ingress/egress costs.**_

### Example Configuration breakpoints
#### [Up to 1000 hosts](https://calculator.aws/#/estimate?id=ae7d7ddec64bb979f3f6611d23616b1dff0e8dbd)

| Fleet instances | CPU Units     | RAM |
|-----------------|---------------|-----|
| 1 Fargate task  | 512 CPU Units | 4GB |

| Dependencies | Version                 | Instance type |
|--------------|-------------------------|---------------|
| Redis        | 6                       | t4g.small     |
| MySQL        | 5.7.mysql_aurora.2.10.0 | db.t3.small   |        

#### [Up to 25000 hosts](https://calculator.aws/#/estimate?id=4a3e3168275967d1e79a3d1fcfedc5b17d67a271)

| Fleet instances | CPU Units     | RAM |
|-----------------|---------------|-----|
| 10 Fargate task  | 1024 CPU Units | 4GB |

| Dependencies | Version                 | Instance type |
|--------------|-------------------------|---------------|
| Redis        | 6                       |  m6g.large    |
| MySQL        | 5.7.mysql_aurora.2.10.0 | db.r6g.large  |


#### [Up to 150000 hosts](https://calculator.aws/#/estimate?id=6a852ef873c0902f0c953045dec3e29fcd32aef8)

| Fleet instances | CPU Units      | RAM |
|-----------------|----------------|-----|
| 30 Fargate task | 1024 CPU Units | 4GB |

| Dependencies | Version                 | Instance type  | Nodes |
|--------------|-------------------------|----------------|-------|
| Redis        | 6                       | m6g.large      | 3     |
| MySQL        | 5.7.mysql_aurora.2.10.0 | db.m6g.8xlarge | 1     |


## Cloud Providers

### AWS

AWS reference architecture can be found [here](https://github.com/fleetdm/fleet/tree/main/tools/terraform). This configuration includes:

- VPC
  - Subnets
    - Public & Private
  - ACLs
  - Security Groups
- ECS as the container orchestrator
  - Fargate for underlying compute
  - Task roles via IAM
- RDS Aurora MySQL 5.7
- Elasticache Redis Engine
- Firehose osquery log destination
  - S3 bucket sync to allow further ingestion/processing
- [Monitoring via Cloudwatch alarms](https://github.com/fleetdm/fleet/tree/main/tools/terraform/monitoring)

Some AWS services used in the provider reference architecture are billed as pay-per-use such as Firehose. This means that osquery scheduled query frequency can have
a direct correlation to how much these services cost, something to keep in mind when configuring Fleet in AWS.

#### AWS Terraform CI/CD IAM Permissions
The following permissions are the minimum required to apply AWS terraform resources:
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ec2:*",
                "cloudwatch:*",
                "s3:*",
                "lambda:*",
                "ecs:*",
                "rds:*",
                "rds-data:*",
                "secretsmanager:*",
                "pi:*",
                "ecr:*",
                "iam:*",
                "aps:*",
                "vpc:*",
                "kms:*",
                "elasticloadbalancing:*",
                "ce:*",
                "cur:*",
                "logs:*",
                "cloudformation:*",
                "ssm:*",
                "sns:*",
                "elasticache:*",
                "application-autoscaling:*",
                "acm:*",
                "route53:*",
                "dynamodb:*",
                "kinesis:*",
                "firehose:*"
            ],
            "Resource": "*"
        }
    ]
}
```

### GCP

Coming soon

### Azure

Coming soon

### Render

Using [Render's IAC](https://render.com/docs/infrastructure-as-code) see [the repository](https://github.com/edwardsb/fleet-on-render) for full details.
```yaml
services:
  - name: fleet
    plan: standard
    type: web
    env: docker
    healthCheckPath: /healthz
    envVars:
      - key: FLEET_MYSQL_ADDRESS
        fromService:
          name: fleet-mysql
          type: pserv
          property: hostport
      - key: FLEET_MYSQL_DATABASE
        fromService:
          name: fleet-mysql
          type: pserv
          envVarKey: MYSQL_DATABASE
      - key: FLEET_MYSQL_PASSWORD
        fromService:
          name: fleet-mysql
          type: pserv
          envVarKey: MYSQL_PASSWORD
      - key: FLEET_MYSQL_USERNAME
        fromService:
          name: fleet-mysql
          type: pserv
          envVarKey: MYSQL_USER
      - key: FLEET_REDIS_ADDRESS
        fromService:
          name: fleet-redis
          type: pserv
          property: hostport
      - key: FLEET_SERVER_TLS
        value: false
      - key: PORT
        value: 8080

  - name: fleet-mysql
    type: pserv
    env: docker
    repo: https://github.com/render-examples/mysql
    branch: mysql-5
    disk:
      name: mysql
      mountPath: /var/lib/mysql
      sizeGB: 10
    envVars:
      - key: MYSQL_DATABASE
        value: fleet
      - key: MYSQL_PASSWORD
        generateValue: true
      - key: MYSQL_ROOT_PASSWORD
        generateValue: true
      - key: MYSQL_USER
        value: fleet

  - name: fleet-redis
    type: pserv
    env: docker
    repo: https://github.com/render-examples/redis
    disk:
      name: redis
      mountPath: /var/lib/redis
      sizeGB: 10
```

### Digital Ocean

Using Digital Ocean's [App Spec](https://docs.digitalocean.com/products/app-platform/concepts/app-spec/) to deploy on the App on the [App Platform](https://docs.digitalocean.com/products/app-platform/)
```yaml
alerts:
- rule: DEPLOYMENT_FAILED
- rule: DOMAIN_FAILED
databases:
- cluster_name: fleet-redis
  engine: REDIS
  name: fleet-redis
  production: true
  version: "6"
- cluster_name: fleet-mysql
  db_name: fleet
  db_user: fleet
  engine: MYSQL
  name: fleet-mysql
  production: true
  version: "8"
domains:
- domain: demo.fleetdm.com
  type: PRIMARY
envs:
- key: FLEET_MYSQL_ADDRESS
  scope: RUN_TIME
  value: ${fleet-mysql.HOSTNAME}:${fleet-mysql.PORT}
- key: FLEET_MYSQL_PASSWORD
  scope: RUN_TIME
  value: ${fleet-mysql.PASSWORD}
- key: FLEET_MYSQL_USERNAME
  scope: RUN_TIME
  value: ${fleet-mysql.USERNAME}
- key: FLEET_MYSQL_DATABASE
  scope: RUN_TIME
  value: ${fleet-mysql.DATABASE}
- key: FLEET_REDIS_ADDRESS
  scope: RUN_TIME
  value: ${fleet-redis.HOSTNAME}:${fleet-redis.PORT}
- key: FLEET_SERVER_TLS
  scope: RUN_AND_BUILD_TIME
  value: "false"
- key: FLEET_REDIS_PASSWORD
  scope: RUN_AND_BUILD_TIME
  value: ${fleet-redis.PASSWORD}
- key: FLEET_REDIS_USE_TLS
  scope: RUN_AND_BUILD_TIME
  value: "true"
jobs:
- envs:
  - key: DATABASE_URL
    scope: RUN_TIME
    value: ${fleet-redis.DATABASE_URL}
  image:
    registry: fleetdm
    registry_type: DOCKER_HUB
    repository: fleet
    tag: latest
  instance_count: 1
  instance_size_slug: basic-xs
  kind: PRE_DEPLOY
  name: fleet-migrate
  run_command: fleet prepare --no-prompt=true db
  source_dir: /
name: fleet
region: nyc
services:
- envs:
  - key: FLEET_VULNERABILITIES_DATABASES_PATH
    scope: RUN_TIME
    value: /home/fleet
  - key: FLEET_BETA_SOFTWARE_INVENTORY
    scope: RUN_TIME
    value: "1"
  health_check:
    http_path: /healthz
  http_port: 8080
  image:
    registry: fleetdm
    registry_type: DOCKER_HUB
    repository: fleet
    tag: latest
  instance_count: 1
  instance_size_slug: basic-xs
  name: fleet
  routes:
  - path: /
  run_command: fleet serve
  source_dir: /
```