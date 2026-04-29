# https://manpages.debian.org/testing/dpkg-dev/deb822.5.en.html

# given a stream of deb822-formatted input lines, this outputs a stream of parsed objects (like "deb822_parse" below, but in streaming form)
#
#   jq --raw-input --null-input 'include "deb822"; deb822_stream(inputs) | ...'
#
def deb822_stream(lines):
	foreach (
		lines,
		"" # inject a synthetic blank line at the end of the input stream to make sure we output everything (because we only output on empty lines, when we know an "entry" is done)
		| select(
			# ignore comment lines (optional in the spec, but for documents that should not have them they are invalid syntax anyhow so should be fairly harmless to strip unilaterally)
			startswith("#")
			| not
			# TODO consider splitting this into a separate function, like "filter_inline_pgp_noise" ?
		)
	) as $line ({ accum: {} };
		if $line == "" then
			{ out: .accum, accum: {} }
		else # TODO should we throw an error if a line contains a newline? (that's bad input)
			def _trimstart: until(startswith(" ") or startswith("\t") | not; .[1:]);
			def _trimend: until(endswith(" ") or endswith("\t") | not; .[:-1]);
			del(.out)
			| ($line | _trimstart) as $ltrim
			| ($ltrim | _trimend) as $trim
			| if $ltrim != $line then
				.accum[.cur] += "\n" + $trim
			else
				($trim | index(":")) as $colon
				| if $colon then
					.cur = $trim[:$colon]
					| .accum[.cur] = ($trim[$colon+1:] | _trimstart)
				else . end # ignore malformed lines that miss a colon
			end
		end
		;
		if .out and (.out | length) > 0 then .out else empty end
	)
;

