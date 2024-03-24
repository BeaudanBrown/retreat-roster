FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
RUN mkdir -p ./data
COPY .prodenv ./.env
COPY ./www ./www
COPY ./dist/main .
EXPOSE 6969
CMD ["./main"]
