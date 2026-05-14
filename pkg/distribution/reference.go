package distribution

import "strings"

// ImageName extracts the short image name from a repository path or full reference.
// e.g. "library/nginx" → "nginx", "registry.com/ns/app:1.0" → "app"
func ImageName(repo string) string {
	// Strip digest if present
	if idx := strings.LastIndex(repo, "@"); idx != -1 {
		repo = repo[:idx]
	}
	// Strip tag if present
	if idx := strings.LastIndex(repo, ":"); idx > strings.LastIndex(repo, "/") {
		repo = repo[:idx]
	}
	idx := strings.LastIndex(repo, "/")
	if idx == -1 {
		return repo
	}
	return repo[idx+1:]
}

// ImagePath extracts the repository path after the source registry host from a
// full image reference. If no registry host is detected, it returns the entire
// repository path.
// e.g. "registry.com/ns/app:1.0" → "ns/app", "library/nginx:latest" → "library/nginx"
func ImagePath(ref string) string {
	// Strip digest if present (e.g. @sha256:abc123)
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}

	// Strip tag if present
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		ref = ref[:idx]
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		return ref
	}

	// If the first component contains a dot or colon, it's a registry host
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
		return parts[1]
	}
	return ref
}

// StripTag removes the tag from a full image reference. Digest references are
// left unchanged.
func StripTag(ref string) string {
	// Do not strip digest references
	if strings.Contains(ref, "@") {
		return ref
	}
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		return ref[:idx]
	}
	return ref
}
