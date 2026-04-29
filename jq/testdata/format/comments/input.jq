# parse a stream of lines into records
def parse_stream(lines):
	foreach (
		lines,
		"" # inject a synthetic blank line at the end
	) as $line ({ accum: {} };
		if $line == "" then
			{ out: .accum, accum: {} }
		else
			.accum[.cur] = $line
		end
		;
		if .out then .out else empty end # trailing comma trick
	)
;
