#!/bin/sh
go mod download
go install
sudo cp rpb.service /etc/systemd/system/
sudo systemctl daemon-reload