FROM golang:latest

WORKDIR /RaftMsgStream

COPY go.mod go.sum ./
RUN go mod tidy

COPY ./raft ./raft
COPY ./server ./server
COPY ./models ./models
COPY ./templates ./templates
COPY ./static ./static
COPY ./launchers/server/main.go ./

ENV SERVER_PORT=8080
ENV RAFT_PORT=5001

RUN go get google.golang.org/protobuf
RUN go get github.com/gin-gonic/gin

RUN go build -o serverLauncher main.go

ENTRYPOINT ["./serverLauncher"]
