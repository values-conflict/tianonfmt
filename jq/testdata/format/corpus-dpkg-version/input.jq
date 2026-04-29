# https://www.debian.org/doc/debian-policy/ch-controlfields.html#version
# given a Debian version, returns a parsed object: { epoch, upstream, revision }
def dpkg_version_parse:
	capture("^(?:(?<epoch>[0-9]*)(?:[:])|)(?<upstream>.*?)(?:[-](?<revision>[^-]*))?$")
;

# given a parsed object (from dpkg_version_parse), returns a Debian version string
def dpkg_version_string:
	if .epoch then "\(.epoch):" else "" end
	+ .upstream
	+ if .revision then "-\(.revision)" else "" end
;
