package backend

type ProviderConfig struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type WorkspaceConfig struct {
	Path    string   `json:"path"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type WorkerConfig struct {
	MaxReviewLoops    int           `json:"maxReviewLoops"`
	Planner           PlannerConfig `json:"planner"`
	SummarizeOnSuccess bool         `json:"summarizeOnSuccess"`
}

type PlannerConfig struct {
	Auto bool `json:"auto"`
}

type Config struct {
	Provider   ProviderConfig                  `json:"provider"`
	Workspace  WorkspaceConfig                 `json:"workspace"`
	Permission map[string]map[string]string    `json:"permission"`
	Worker     WorkerConfig                    `json:"worker"`
}

type Workspace struct {
	Root    string   `json:"root"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type DoctorPayload struct {
	Root       string                 `json:"root"`
	ConfigPath string                 `json:"configPath"`
	Provider   ProviderConfig         `json:"provider"`
	Workspace  WorkspaceConfig        `json:"workspace"`
	Permission map[string]map[string]string `json:"permission"`
	Worker     WorkerConfig           `json:"worker"`
}

type RunResult struct {
	Status  string         `json:"status"`
	Goal    string         `json:"goal"`
	Planned bool           `json:"planned"`
	Tasks   []RunTask      `json:"tasks"`
	Summary *SummarizerOutput `json:"summary,omitempty"`
	LogPath string         `json:"log_path"`
}

type RunTask struct {
	Task              string             `json:"task"`
	Executor          ExecutorOutput     `json:"executor"`
	AppliedOperations []AppliedOperation `json:"applied_operations,omitempty"`
	Reviewer          ReviewerOutput     `json:"reviewer"`
}

type AppliedOperation struct {
	Tool    string      `json:"tool"`
	Path    string      `json:"path,omitempty"`
	Result  any         `json:"result,omitempty"`
	Content string      `json:"content,omitempty"`
}

type PlannerTask struct {
	ID        string   `json:"id"`
	Description string `json:"description"`
	DoneWhen  []string `json:"done_when"`
	DependsOn []string `json:"depends_on"`
}

type PlannerPhase struct {
	Name  string        `json:"name"`
	Goal  string        `json:"goal"`
	Tasks []PlannerTask `json:"tasks"`
}

type PlannerOutput struct {
	Status     string         `json:"status"`
	Summary    string         `json:"summary"`
	Phases     []PlannerPhase `json:"phases"`
	NextAction string         `json:"next_action"`
}

type ExecutorOperation struct {
	Tool    string `json:"tool"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Patch   string `json:"patch,omitempty"`
}

type ExecutorOutput struct {
	Status        string              `json:"status"`
	Summary       string              `json:"summary"`
	Changes       []string            `json:"changes"`
	Assumptions   []string            `json:"assumptions"`
	OpenQuestions []string            `json:"open_questions"`
	Operations    []ExecutorOperation `json:"operations"`
	NextAction    string              `json:"next_action"`
}

type ReviewError struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ReviewerOutput struct {
	Status          string        `json:"status"`
	Summary         string        `json:"summary"`
	Errors          []ReviewError `json:"errors"`
	VerifiedAgainst []string      `json:"verified_against"`
	NextAction      string        `json:"next_action"`
}

type SummarizerOutput struct {
	Status       string `json:"status"`
	ProjectState string `json:"project_state"`
	NextAction   string `json:"next_action"`
}

type AgentInfo struct {
	Role    string
	Slug    string
	Name    string
	Purpose string
}

var agents = map[string]AgentInfo{
	"planner": {
		Role: "planner", Slug: "metre", Name: "Metre", Purpose: "descompone objetivos en fases y tareas",
	},
	"executor": {
		Role: "executor", Slug: "cocinero", Name: "Cocinero", Purpose: "implementa y corrige",
	},
	"reviewer": {
		Role: "reviewer", Slug: "auditor", Name: "Auditor", Purpose: "revisa con criterio estricto",
	},
	"summarizer": {
		Role: "summarizer", Slug: "caja", Name: "Caja", Purpose: "condensa el estado del proyecto",
	},
}

func getAgent(role string) AgentInfo {
	agent, ok := agents[role]
	if !ok {
		panic("agente no soportado: " + role)
	}
	return agent
}
