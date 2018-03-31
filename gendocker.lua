local gensh = "set -e\n"

local dftmpl = [[
FROM golang:alpine
RUN apk --no-cache add git
ADD . /root/dcdn
WORKDIR /root/dcdn
%s
RUN (cd %s && go build)

FROM alpine:latest
RUN apk --no-cache add ca-certificates && adduser -D %s
COPY --from=0 /root/dcdn/%s/%s /bin/%s
USER %s
WORKDIR /home/%s
%s
ENTRYPOINT ["/bin/%s"]
]]

local function gend(name, path, gget, sql)
    if path == nil then
        path = name
    end
    if gget == nil then
        gget = ""
    else
        gget = "RUN go get " .. gget
    end
    if sql then
        sql = "COPY discovery/gentbl.sql gentbl.sql"
    else
        sql = ""
    end
    local df = string.format(dftmpl, gget, path, name, path, name, name, name, name, sql, name)
    local f = assert(io.open(name .. ".dockerfile", "w+"))
    f:write(df)
    f:close()
    gensh = gensh .. string.format("docker build -f %s.dockerfile -t dcdn/%s .\n", name, name)
end

gend("dcdncache")
gend("dcdnproxy", nil, "github.com/elazarl/goproxy")
gend("dcdnserver")
gend("checker", "discovery/checker")
gend("pruner", "discovery/pruner", "github.com/lib/pq", true)
gend("discovery", "discovery/discovery", "github.com/lib/pq github.com/cridenour/go-postgis", true)

local f = assert(io.open("gen.sh", "w+"))
f:write(gensh)
f:close()
