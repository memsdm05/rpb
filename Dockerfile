FROM golang:1.22.5-alpine AS build

ENV GOCACHE=/root/.cache/go-build
ENV CGO_ENABLED=1

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN --mount=type=cache,target="/root/.cache/go-build" go build -v -o rpb ./...
    
FROM alpine:latest

COPY --from=build /app/rpb /app/rpb
WORKDIR /app

ENTRYPOINT [ "./rpb" ]