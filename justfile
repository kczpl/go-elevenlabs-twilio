default:
  @just --list

build:
  GOOS=linux GOARCH=amd64 go build -o caller-api cmd/main.go