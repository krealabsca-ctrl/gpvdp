# ---- build ----
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
# Cachea dependencias primero
COPY go.mod ./
COPY go.sum* ./
RUN go mod download
# Copia el resto y compila
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/api  ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/seed ./cmd/seed

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/api  /app/api
COPY --from=build /out/seed /app/seed
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
