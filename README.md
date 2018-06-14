# cf-report-users

CloudFoundry CLI plugin to list all users in a CloudFoundry installation

## Install from binary

Pick as appropriate for your OS:

```bash
cf install-plugin https://github.com/govau/cf-report-users/releases/download/v0.6.0/report-users.linux32
cf install-plugin https://github.com/govau/cf-report-users/releases/download/v0.6.0/report-users.linux64
cf install-plugin https://github.com/govau/cf-report-users/releases/download/v0.6.0/report-users.osx
cf install-plugin https://github.com/govau/cf-report-users/releases/download/v0.6.0/report-users.win32
cf install-plugin https://github.com/govau/cf-report-users/releases/download/v0.6.0/report-users.win64
```

## Usage

```bash
cf report-users
```

## Development

```bash
go install ./cmd/report-users && \
    cf install-plugin $GOPATH/bin/report-users -f && \
    cf report-users
```

## Building a new release

```bash
PLUGIN_PATH=$GOPATH/src/github.com/govau/cf-report-users/cmd/report-users
PLUGIN_NAME=$(basename $PLUGIN_PATH)

GOOS=linux GOARCH=amd64 go build -o ${PLUGIN_NAME}.linux64 cmd/${PLUGIN_NAME}/${PLUGIN_NAME}.go
GOOS=linux GOARCH=386 go build -o ${PLUGIN_NAME}.linux32 cmd/${PLUGIN_NAME}/${PLUGIN_NAME}.go
GOOS=windows GOARCH=amd64 go build -o ${PLUGIN_NAME}.win64 cmd/${PLUGIN_NAME}/${PLUGIN_NAME}.go
GOOS=windows GOARCH=386 go build -o ${PLUGIN_NAME}.win32 cmd/${PLUGIN_NAME}/${PLUGIN_NAME}.go
GOOS=darwin GOARCH=amd64 go build -o ${PLUGIN_NAME}.osx cmd/${PLUGIN_NAME}/${PLUGIN_NAME}.go

shasum -a 1 ${PLUGIN_NAME}.*
```
