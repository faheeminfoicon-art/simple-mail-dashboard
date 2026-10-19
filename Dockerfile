FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/app .

FROM alpine:3.22

WORKDIR /app

COPY --from=build /app/app /app/app
COPY static ./static

RUN mkdir -p /var/data

ENV PORT=10000
ENV DATA_FILE=/var/data/store.json

EXPOSE 10000

CMD ["/app/app"]
