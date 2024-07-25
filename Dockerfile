FROM golang:1.22.5-alpine AS build

WORKDIR /app

RUN apk add --update gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -v -o rpb ./...
    
FROM alpine:latest

COPY --from=build /app/rpb /app/rpb
WORKDIR /app

ENTRYPOINT [ "./rpb" ]