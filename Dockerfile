FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Australia/Melbourne
WORKDIR /root/
RUN mkdir -p ./data
COPY ./www ./www
COPY ./dist/main .
EXPOSE 6969
CMD ["./main"]
