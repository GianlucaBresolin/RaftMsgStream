#!/bin/bash

sudo apt-get update -y
sudo apt-get install -y docker.io

sudo systemctl start docker
sudo systemctl enable docker

sudo docker pull gianlucabresolin/raftmsgserver

sudo docker run --name raftmsgserver -p 5001:5001 -p 8080:8080 -d gianlucabresolin/raftmsgserver node address voting other-node other-node-address