foreach .[] as $item (
	{ count: 0, sum: 0 };
	{ count: (.count + 1), sum: (.sum + $item) }
; .sum / .count)
