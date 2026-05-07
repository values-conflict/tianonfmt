if
	any(.; (
		"prefix-a/",
		"prefix-b/",
		empty
	) as $p | startswith($p))
	| not
then
	.items
end
