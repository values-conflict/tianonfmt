module {};

import "bar" as $bar;

def f($x; $y):
	$x | not | $y
;

def g:
	def h: . + 1;
	h
;

label $lbl |
foreach .[] as $item (
	0;
	. + $item
; . * 2)
| reduce .[] as $x (
	0;
	. + $x
)
| if . then .a elif .b then .c else .d end
| try .e catch .f | . as $y
| . as [$first, $rest]
| . as {key: $val}
| .g[0:2]
| .h[.i]
| @base64
| [.j, .k, empty]
| { l: .m, "n.o": .p, (.q): .r }
| (.s) | .t? | $__loc__ | break $lbl
