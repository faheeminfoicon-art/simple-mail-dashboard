FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app .
FROM alpine:3.22
WORKDIR /app
COPY --from=build /app /app/app
COPY static ./static
RUN mkdir -p /var/data
ENV PORT=10000 DATA_FILE=/var/data/store.json
EXPOSE 10000
CMD ["/app/app"]
