#!/bin/sh
go mod download
go install -v
sudo cp rpb.service /etc/systemd/system/
sudo systemctl daemon-reload
ln -s $GOPATH/bin/rpb /usr/local/bin/rpb
mkdir /etc/rpb
cp .env /etc/rpb