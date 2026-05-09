# pipe chain where a commented step wraps a multi-line comma —
# exercises pipeChainMultiLine's pipePartIsMultiLineComma indent/dedent path
.items[]
# select these fields
| .name, .version, .description, .maintainer, .url, empty
