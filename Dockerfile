FROM golang:1.15-buster 

WORKDIR /go
COPY  main .
COPY  tpl ./tpl
ENTRYPOINT [ "./main" ]


