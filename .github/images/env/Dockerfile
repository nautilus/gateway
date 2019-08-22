FROM golang:latest

RUN go get github.com/aleciavazis/run

RUN run install:ci

ADD entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]