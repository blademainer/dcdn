FROM alpine:latest
RUN apk --no-cache add lua docker
COPY . /root/dcdn
WORKDIR /root/dcdn
RUN lua gendocker.lua
ENTRYPOINT ["sh", "gen.sh"]
