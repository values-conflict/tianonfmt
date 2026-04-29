def normalize:
	gsub("^\\s+|\\s+$"; "")
	| gsub("\\s+"; " ")
;

def normalize_ref_to_docker:
	ltrimstr("docker.io/")
	| ltrimstr("library/")
;

.images | map(normalize_ref_to_docker)
