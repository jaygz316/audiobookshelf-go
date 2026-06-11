#!/bin/sh

# Mark the working directory as safe for use with git
git config --global --add safe.directory $PWD

# Download Go modules
go mod download
