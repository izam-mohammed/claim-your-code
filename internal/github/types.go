package github

// RepoInfo represents a GitHub repository.
type RepoInfo struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
	Archived      bool   `json:"archived"`
}

// PRInfo represents a GitHub pull request.
type PRInfo struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	BaseBranch string `json:"base_branch"`
	Repo       RepoInfo
}

// CommitInfo represents a GitHub commit.
type CommitInfo struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// UserInfo represents a GitHub user.
type UserInfo struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}
