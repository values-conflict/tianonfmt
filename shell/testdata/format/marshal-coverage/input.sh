#!/usr/bin/env bash
(( x++ ))
[[ -f /etc/foo ]]
let x=5 y=x+1
time sleep 0
! false &
for x in a b c; do
	echo "$x"
done
while read -r line; do
	echo "$line"
done
case "$x" in
	a) echo a ;;
	b) echo b ;;
	*) echo other ;;
esac
(cd /tmp && echo hi)
{ echo a; echo b; }
echo hi >/dev/null
