FROM golang:1.22.5-alpine AS build

WORKDIR /app

COPY . .

RUN apk add --update gcc musl-dev

RUN go mod download

RUN go build -v -o rpb .
    
FROM alpine:latest

COPY --from=build /app/rpb /app/rpb
WORKDIR /app

ENTRYPOINT [ "./rpb" ]