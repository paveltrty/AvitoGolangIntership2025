FROM golang:1.23 as builder

WORKDIR /app

ENV GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o reviewer-service .

FROM alpine:3.20

RUN adduser -D -g '' appuser
USER appuser

WORKDIR /home/appuser

COPY --from=builder /app/reviewer-service /home/appuser/reviewer-service

EXPOSE 8080

ENTRYPOINT ["./reviewer-service"]

