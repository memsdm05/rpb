
<img src="https://github.com/memsdm05/rpb/blob/master/rpbibi.png?raw=true" width="200"/>


# Remote Power Button
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/memsdm05/rpb)
![GitHub License](https://img.shields.io/github/license/memsdm05/rpb)
[![Go Report Card](https://goreportcard.com/badge/github.com/memsdm05/rpb)](https://goreportcard.com/report/github.com/memsdm05/rpb)


Control a power button from **anywhere** in the world!

> Now version 3!



## Background

This project was convieved to control the power state of my desktop via a web interface. It uses a Raspberry Pi that both controls a relay board and gets the state of the status LED. This is the first real implementation, the other two where crap.


## Getting Started
It's preferred to run the server via Docker. A docker compose is provided for convenience.
1. ```cp .env.example .env```
2. In .env, change `RPB_SECRET` to something else
3. Run ```docker compose up -d```


By default, rpb runs on port **5000**. If Docker doesn't work for you, you can also use the systemd service file.