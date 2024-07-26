#!/bin/sh
go mod download
go install -v
sudo cp rpb.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo ln -s $GOPATH/bin/rpb /usr/local/bin/rpb
sudo mkdir /etc/rpb
sudo cp .env /etc/rpb