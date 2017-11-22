# cf-run-and-wait

CloudFoundry CLI plugin to list all users in a CloudFoundry installation

## Installing

```bash
go get github.com/govau/cf-report-users/cmd/report-users
cf install-plugin $GOPATH/bin/report-users -f
```

## Running a task

```bash
cf report-users
```

If successful, will exit with status code of `0`.

If it fails, will print some debug info, and exit with non-zero status code.

## Building a new release

```bash
PLUGIN_PATH=$GOPATH/src/github.com/govau/cf-report-users/cmd/report-users
PLUGIN_NAME=$(basename $PLUGIN_PATH)
cd $PLUGIN_PATH

GOOS=linux GOARCH=amd64 go build -o ${PLUGIN_NAME}.linux64
GOOS=linux GOARCH=386 go build -o ${PLUGIN_NAME}.linux32
GOOS=windows GOARCH=amd64 go build -o ${PLUGIN_NAME}.win64
GOOS=windows GOARCH=386 go build -o ${PLUGIN_NAME}.win32
GOOS=darwin GOARCH=amd64 go build -o ${PLUGIN_NAME}.osx

shasum -a 1 ${PLUGIN_NAME}.linux64
shasum -a 1 ${PLUGIN_NAME}.linux32
shasum -a 1 ${PLUGIN_NAME}.win64
shasum -a 1 ${PLUGIN_NAME}.win32
shasum -a 1 ${PLUGIN_NAME}.osx
```
