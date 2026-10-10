#! /bin/bash

echo -e "Start running the script..."
cd ../

echo -e "Current Go version: \c"
go version

echo -e "Install the Wails command line tool..."
go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0

echo -e "Successful installation!"

echo -e "End running the script!"
