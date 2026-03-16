FROM golang:1.21.0 AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /prolinkrobot ./cmd

FROM alpine:3.20

RUN apk add --no-cache tzdata
ENV TZ=Asia/Tashkent

COPY --from=builder /prolinkrobot /prolinkrobot

CMD ["/prolinkrobot"]
