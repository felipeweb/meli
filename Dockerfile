FROM golang AS builder
WORKDIR /app
ENV GO111MODULE=on
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o meli ./cmd/meli

FROM alpine
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/meli /meli
ENV ADDR 0.0.0.0:8080
EXPOSE 8080
CMD [ "sh", "-c", "/meli -addr ${ADDR} -redis_url ${REDIS_URL} -trace_addr ${TRACE_ADDR}" ]