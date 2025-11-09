# govd migration tool
this utility helps you migrate your govd users from v1 (MySQL) to v2 (PostgreSQL).

## run
make sure you have Docker installed and both the source (MySQL) and target (PostgreSQL) databases running and accessible (exposed or shared network), then run:

```bash
docker run --rm \
  -e V1_DSN='govd:password@tcp(db_v1:3306)/govd?charset=utf8mb4&parseTime=True&loc=Local' \
  -e V2_DSN='postgres://govd:password@db_v2:5432/govd?sslmode=disable' \
  govdbot/migrate:main
```