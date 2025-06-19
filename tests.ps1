$dirname = Split-Path -Parent $MyInvocation.MyCommand.Path
$dirname = Split-Path -Leaf $dirname

# Reset test docker containers and volumes
docker-compose -f "./test-databases.docker-compose.yml" down
docker volume rm "${dirname}_queries_mysql_data"
docker volume rm "${dirname}_queries_mariadb_data"
docker volume rm "${dirname}_queries_postgres_data"
docker-compose -f "./test-databases.docker-compose.yml" up -d

# Databases to test (these translate to Go build tags)
$Databases = @(
    "sqlite",
    "mysql_local",
    "mysql",
    "mariadb",
    "postgres"
)

# If arguments are provided, use them as database types
if ($args.count -gt 0) {
    $Databases = @()
    foreach ($arg in $args) {
        $Databases += $arg
    }
}

# Run tests for each database type
foreach ($Database in $Databases) {
    Write-Host "Running tests for $Database"
    go test -tags="$Database" ./... --failfast
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Tests failed for $Database"
        exit $LASTEXITCODE
    }
    Write-Host "Tests passed for $Database"
    Write-Host "----------------------------------------"
}