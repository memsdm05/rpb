FROM golang:1.22.5-alpine AS build

WORKDIR /app

RUN apk add --update gcc musl-dev

COPY . .

RUN go mod download

RUN go build -o rpb .

FROM alpine:latest

COPY --from=build /app/rpb /app/rpb
WORKDIR /app

ENTRYPOINT [ "./rpb" ]