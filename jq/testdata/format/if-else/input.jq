if .arch == "amd64" then
	"x86_64"
elif .arch == "arm64" then
	"aarch64"
else
	error("unsupported arch: \(.arch)")
end
