FROM golang:latest

WORKDIR /RaftMsgStream

COPY go.mod go.sum ./
RUN go mod tidy

COPY ./raft ./raft
COPY ./server ./server
COPY ./client ./client
COPY ./models ./models
COPY ./launchers/server/main.go ./

RUN go get google.golang.org/protobuf

RUN go build -o serverLauncher main.go

ENTRYPOINT ["./serverLauncher"]
