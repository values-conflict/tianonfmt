# label/break: early-exit from a generator — real idiom from openjdk library generation
# Ref: https://github.com/docker-library/openjdk/blob/638eb4b1d59af728d31e031168bd4cafc225f097/generate-stackbrew-library.jq
[
	.[]
	| label $out |
	if . > 5 then ., break $out else . end
]
