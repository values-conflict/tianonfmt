# parse a stream of lines into records
def parse_stream(lines):
	foreach (
		lines,
		"" # inject a synthetic blank line at the end
	) as $line ({ accum: {} };
		if $line == "" then
			{ out: .accum, accum: {} }
		else .accum[.cur] = $line end;
		if .out then .out else empty end # trailing comma trick
	)
;

# pipe part with both leading comment AND trailing comment on same step
# also exercises pipeChainMultiLine's leading+trailing comment path
.data
# leading: filter to active items
| select(.active) # trailing: keep only active
| .name
