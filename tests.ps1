$dirname = Split-Path -Parent $MyInvocation.MyCommand.Path
$dirname = Split-Path -Leaf $dirname

# Reset test docker containers
docker-compose -f "./test-databases.docker-compose.yml" down

# Databases to test (these translate to Go build tags)
$Databases = @(
    "sqlite",
    "mysql_local",
    "mysql",
    "mariadb",
    "postgres"
)

# Databases defined in docker-compose.yml
$DockerDatabases = @{
    "mysql"      = $true
    "mariadb"    = $true
    "postgres"   = $true
}

# Empty test array will be built out
$testsToRun = @(

)

# Flags for the test run
$flags = @{
    verbose = $true
    failslow = $false
}

# Check script arguments that were passed in
foreach ($arg in $args) {
    switch ($arg) {
        "silent" {
            $flags.verbose = $false
            continue
        }
        "failslow" {
            $flags.failslow = $false
            continue
        }
        "down" {
            # If the argument is "down", remove all volumes and exit
            foreach ($db in $DockerDatabases.Keys) {
                docker volume rm "${dirname}_queries_${db}_data" -f
            }
            exit 0
        }
        default {
            $testsToRun += $arg
        }
    }
}

if ($testsToRun.Count -eq 0) {
    # If no specific tests were provided, run all databases
    $testsToRun = $Databases
}

# Run tests for each database type
foreach ($Database in $testsToRun) {

    # Check if the argument is a valid Docker database type
    # if it is, reset the corresponding Docker volume and start the container
    if ($DockerDatabases.ContainsKey($Database)) {
        docker volume rm "${dirname}_queries_${Database}_data"
        docker-compose -f "./test-databases.docker-compose.yml" up $Database -d
    }

    $cmd = "go test -tags=`"$Database`" --timeout=30s"
    if ($flags.verbose) {
        $cmd += " -v"
    }
    if ($flags.failslow -eq $false) {
        $cmd += " --failfast"
    }
    
    $cmd += " ./..."

    Write-Host "Running tests for $Database"
    Write-Host "Command: $cmd"
    Write-Host "----------------------------------------"
    Invoke-Expression $cmd
    if ($LASTEXITCODE -ne 0) {
        Write-Host "----------------------------------------"
        Write-Host "Tests failed for $Database"
        exit $LASTEXITCODE
    }
    Write-Host "Tests passed for $Database"
    Write-Host "----------------------------------------"
}