#!/bin/sh
go mod download
go install -v
sudo cp rpb.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo ln -s $GOPATH/bin/rpb /usr/local/bin/rpb || true
sudo mkdir /etc/rpb || true
sudo cp .env /etc/rpb || true