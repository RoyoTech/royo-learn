package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/buildinfo"
	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/curate"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/publish"
	"agent-royo-learn/internal/recurrence"
	"agent-royo-learn/internal/storage"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Service wrappers — thin adapters for services used by tools.
// ---------------------------------------------------------------------------

type captureSvc struct {
	db         *storage.DB
	recordsDir string
	evidence   *evidence.Service
}

// newCaptureSvc wires capture to the evidence layer. Both are needed: a capture
// service without evidence cannot produce a learning that satisfies the D3
// approval threshold, which is the root defect Recorrido B repairs.
func newCaptureSvc(db *storage.DB, recordsDir, projectRoot string, allowedCommands []string) (*captureSvc, error) {
	ev, err := evidence.NewService(projectRoot, allowedCommands)
	if err != nil {
		return nil, err
	}
	return &captureSvc{db: db, recordsDir: recordsDir, evidence: ev}, nil
}

func (s *captureSvc) service() *capture.Service {
	return capture.NewServiceWithEvidence(s.db, s.recordsDir, s.evidence)
}

func (s *captureSvc) Capture(ctx context.Context, projectID domain.ProjectID, input *capture.CaptureInput) (*capture.CaptureResult, error) {
	return s.service().Capture(ctx, projectID, input)
}

func (s *captureSvc) AddEvidence(ctx context.Context, projectID domain.ProjectID, input *capture.AddEvidenceInput) (*capture.AddEvidenceResult, error) {
	return s.service().AddEvidence(ctx, projectID, input)
}

type curateSvc struct {
	db         *storage.DB
	recordsDir string
}

func newCurateSvc(db *storage.DB, recordsDir string) *curateSvc {
	return &curateSvc{db: db, recordsDir: recordsDir}
}

func (s *curateSvc) Curate(ctx context.Context, projectID domain.ProjectID, input *curate.CurateInput) (*curate.CurateResult, error) {
	svc := curate.NewService(s.db, s.recordsDir)
	return svc.Curate(ctx, projectID, input)
}

type publishSvc struct {
	db          *storage.DB
	projectRoot string
}

func newPublishSvc(db *storage.DB, projectRoot, journalDir string) *publishSvc {
	return &publishSvc{db: db, projectRoot: projectRoot}
}

func (s *publishSvc) Preview(ctx context.Context, projectID domain.ProjectID, input *publish.PreviewInput) (*publish.PreviewResult, error) {
	svc := publish.NewService(s.db, s.projectRoot,
		s.projectRoot+"/.royo-learn/backups",
		s.projectRoot+"/.royo-learn")
	return svc.Preview(ctx, projectID, input)
}

// ---------------------------------------------------------------------------
// Tool input types — Go structs for automatic JSON Schema generation via AddTool.
// ---------------------------------------------------------------------------

type captureLearningInput struct {
	Title           string              `json:"title" jsonschema:"required,title of the learning (5-160 chars)"`
	Type            string              `json:"type" jsonschema:"required,learning type: procedure, prevention, diagnostic, tooling, architecture, quality, security, hypothesis, preference"`
	Context         string              `json:"context" jsonschema:"required,context where the learning occurred"`
	Observation     string              `json:"observation" jsonschema:"required,what was observed"`
	ReusableLesson  string              `json:"reusable_lesson" jsonschema:"required,reusable lesson learned"`
	ScopeGuess      string              `json:"scope_guess" jsonschema:"required,scope: project, shared, personal, unknown"`
	Confidence      string              `json:"confidence" jsonschema:"required,confidence: low, medium, high"`
	EvidenceLevel   string              `json:"evidence_level" jsonschema:"required,evidence level: strong, moderate, weak, insufficient"`
	Actor           actorInput          `json:"actor" jsonschema:"required,who performed the action"`
	RecommendedProc []string            `json:"recommended_procedure,omitempty"`
	Limits          string              `json:"limits,omitempty"`
	ProposedDest    string              `json:"proposed_destination,omitempty" jsonschema:"proposed destination: none, project, shared, skill, agents_rule"`
	RetrievalTerms  []string            `json:"retrieval_terms,omitempty"`
	Evidence        []evidenceItemInput `json:"evidence,omitempty" jsonschema:"evidence records persisted together with the learning; approval requires at least one"`
	IdempotencyKey  string              `json:"idempotency_key,omitempty" jsonschema:"the same key on a retry returns the existing learning and does not duplicate its evidence"`
}

// evidenceItemInput is one element of an evidence[] array.
//
// `kind` is canonical (docs/03). `type` is accepted as an input alias because
// the recovery plan writes the payload that way; both map to the same domain
// EvidenceKind.
type evidenceItemInput struct {
	Kind    string `json:"kind,omitempty" jsonschema:"evidence kind: file, git_diff, git_commit, command, test, engram_observation, issue, pull_request, text, external_reference"`
	Type    string `json:"type,omitempty" jsonschema:"alias of kind"`
	Summary string `json:"summary" jsonschema:"required,human-readable summary of the record"`
	Source  string `json:"source,omitempty" jsonschema:"origin of the record: a path, a command or a URL"`
	Content string `json:"content,omitempty" jsonschema:"literal content; stored in the content-addressed blob store after redaction"`
}

type addEvidenceInput struct {
	LearningID    string              `json:"learning_id" jsonschema:"required,learning to attach the evidence to"`
	Evidence      []evidenceItemInput `json:"evidence" jsonschema:"required,evidence records to attach; at least one"`
	EvidenceLevel string              `json:"evidence_level,omitempty" jsonschema:"optionally raise the declared evidence level: strong, moderate, weak, insufficient"`
	Actor         actorInput          `json:"actor" jsonschema:"required"`
}

// toEvidenceItems maps the wire form onto domain evidence items, rejecting any
// unknown kind rather than silently coercing it.
func toEvidenceItems(in []evidenceItemInput) ([]evidence.Item, error) {
	if len(in) == 0 {
		return nil, nil
	}
	items := make([]evidence.Item, 0, len(in))
	for i, raw := range in {
		kind := raw.Kind
		if kind == "" {
			kind = raw.Type
		}
		if kind == "" {
			kind = string(domain.KindText)
		}
		if !domain.IsValidEvidenceKind(domain.EvidenceKind(kind)) {
			return nil, fmt.Errorf("evidence[%d]: unknown kind %q", i, kind)
		}
		if raw.Summary == "" {
			return nil, fmt.Errorf("evidence[%d]: summary is required", i)
		}
		items = append(items, evidence.Item{
			Kind:    domain.EvidenceKind(kind),
			Summary: raw.Summary,
			Source:  raw.Source,
			Content: raw.Content,
		})
	}
	return items, nil
}

func evidenceIDStrings(ids []domain.EvidenceID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

type actorInput struct {
	Kind      string `json:"kind" jsonschema:"required,kind: human, agent, system"`
	Name      string `json:"name" jsonschema:"required,actor name"`
	Model     string `json:"model,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type searchLearningsInput struct {
	Query  string `json:"query" jsonschema:"required,search query"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type curateLearningInput struct {
	LearningID string     `json:"learning_id" jsonschema:"required,learning ID to curate"`
	Decision   string     `json:"decision" jsonschema:"required,curation decision"`
	Rationale  string     `json:"rationale" jsonschema:"required,rationale (10-5000 chars)"`
	Actor      actorInput `json:"actor" jsonschema:"required"`
	Area       string     `json:"area,omitempty" jsonschema:"optional,área/skill temática destino explícita; si se omite, se deriva automáticamente de los retrieval_terms"`
}

type previewPublicationInput struct {
	LearningID string     `json:"learning_id" jsonschema:"required"`
	Actor      actorInput `json:"actor" jsonschema:"required"`
}

type publishLearningInput struct {
	LearningID  string     `json:"learning_id" jsonschema:"required"`
	PreviewHash string     `json:"preview_hash" jsonschema:"required"`
	Actor       actorInput `json:"actor" jsonschema:"required"`
}

type listLearningsInput struct {
	Status []string `json:"status,omitempty"`
	Type   []string `json:"type,omitempty"`
	Scope  []string `json:"scope,omitempty"`
	Limit  int      `json:"limit,omitempty"`
	Offset int      `json:"offset,omitempty"`
}

type getLearningInput struct {
	LearningID string `json:"learning_id" jsonschema:"required"`
}

type doctorInput struct{}

type listRecurrencesInput struct {
	LearningID string `json:"learning_id" jsonschema:"required,learning ID"`
	Limit      int    `json:"limit,omitempty"`
}

type computeMetricsInput struct {
	LearningID string `json:"learning_id" jsonschema:"required,learning ID"`
}

// ---------------------------------------------------------------------------
// Tool helpers
// ---------------------------------------------------------------------------

func toolResultJSON(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return toolError("json_error", err.Error())
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func toolError(code, message string) (*mcp.CallToolResult, any, error) {
	errJSON, _ := json.Marshal(map[string]string{
		"code":    code,
		"message": message,
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(errJSON)},
		},
	}, nil, nil
}

func toActor(a actorInput) domain.Actor {
	return domain.Actor{
		Kind:      a.Kind,
		Name:      a.Name,
		Model:     a.Model,
		SessionID: a.SessionID,
	}
}

// ---------------------------------------------------------------------------
// Tool handler functions
//
// The registry that binds these handlers to canonical names, deprecated
// aliases, profiles and annotations lives in profiles.go.
// ---------------------------------------------------------------------------

func handleCaptureLearning(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in captureLearningInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in captureLearningInput) (*mcp.CallToolResult, any, error) {
		dest := domain.DestinationType(in.ProposedDest)
		if in.ProposedDest == "" {
			dest = domain.DestProject
		}

		items, err := toEvidenceItems(in.Evidence)
		if err != nil {
			return toolError("invalid_argument", err.Error())
		}

		capIn := &capture.CaptureInput{
			Title:          in.Title,
			Context:        in.Context,
			Observation:    in.Observation,
			Lesson:         in.ReusableLesson,
			Type:           domain.LearningType(in.Type),
			Scope:          domain.Scope(in.ScopeGuess),
			Destination:    dest,
			Confidence:     domain.Confidence(in.Confidence),
			EvidenceLevel:  domain.EvidenceLevel(in.EvidenceLevel),
			Recommended:    in.RecommendedProc,
			Limits:         in.Limits,
			RetrievalTerms: in.RetrievalTerms,
			Actor:          toActor(in.Actor),
			IdempotencyKey: in.IdempotencyKey,
			Evidence:       items,
		}

		result, err := srv.capSvc.Capture(ctx, srv.projectID, capIn)
		if err != nil {
			return toolError("capture_failed", err.Error())
		}

		return toolResultJSON(map[string]any{
			"learning_id":    string(result.LearningID),
			"status":         string(result.Status),
			"new":            result.New,
			"evidence_count": len(result.EvidenceIDs),
			"evidence_ids":   evidenceIDStrings(result.EvidenceIDs),
			"redacted":       result.Redacted,
		})
	}
}

func handleAddEvidence(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in addEvidenceInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in addEvidenceInput) (*mcp.CallToolResult, any, error) {
		if in.LearningID == "" {
			return toolError("invalid_argument", "learning_id is required")
		}
		items, err := toEvidenceItems(in.Evidence)
		if err != nil {
			return toolError("invalid_argument", err.Error())
		}
		if len(items) == 0 {
			return toolError("invalid_argument", "at least one evidence record is required")
		}

		addIn := &capture.AddEvidenceInput{
			LearningID:    domain.LearningID(in.LearningID),
			Items:         items,
			Actor:         toActor(in.Actor),
			EvidenceLevel: domain.EvidenceLevel(in.EvidenceLevel),
		}

		result, err := srv.capSvc.AddEvidence(ctx, srv.projectID, addIn)
		if err != nil {
			return toolError("add_evidence_failed", err.Error())
		}

		return toolResultJSON(map[string]any{
			"learning_id":    string(result.LearningID),
			"evidence_count": result.Count,
			"evidence_ids":   evidenceIDStrings(result.EvidenceIDs),
			"evidence_level": string(result.EvidenceLevel),
			"redacted":       result.Redacted,
		})
	}
}

func handleSearchLearnings(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in searchLearningsInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in searchLearningsInput) (*mcp.CallToolResult, any, error) {
		if in.Query == "" {
			return toolError("invalid_argument", "query is required")
		}

		results, err := storage.Search(ctx, srv.db, srv.projectID, in.Query)
		if err != nil {
			return toolError("search_failed", err.Error())
		}

		if in.Limit > 0 && in.Limit < len(results) {
			results = results[:in.Limit]
		}

		items := make([]map[string]any, 0, len(results))
		for _, l := range results {
			items = append(items, learningToMap(l))
		}
		return toolResultJSON(items)
	}
}

func handleCurateLearning(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in curateLearningInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in curateLearningInput) (*mcp.CallToolResult, any, error) {
		curIn := &curate.CurateInput{
			LearningID: domain.LearningID(in.LearningID),
			Decision:   domain.CurationDecision(in.Decision),
			Rationale:  in.Rationale,
			Actor:      toActor(in.Actor),
			Area:       in.Area,
		}

		result, err := srv.curateSvc.Curate(ctx, srv.projectID, curIn)
		if err != nil {
			return toolError("curate_failed", err.Error())
		}

		return toolResultJSON(map[string]any{
			"curation_id": string(result.CurationID),
			"learning_id": string(result.LearningID),
			"new_status":  string(result.NewStatus),
		})
	}
}

func handlePreviewPublication(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in previewPublicationInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in previewPublicationInput) (*mcp.CallToolResult, any, error) {
		previewIn := &publish.PreviewInput{
			LearningID: domain.LearningID(in.LearningID),
			Actor:      toActor(in.Actor),
		}

		result, err := srv.publishSvc.Preview(ctx, srv.projectID, previewIn)
		if err != nil {
			return toolError("preview_failed", err.Error())
		}

		return toolResultJSON(map[string]any{
			"preview_id":        string(result.Preview.ID),
			"preview_hash":      result.Preview.PreviewHash,
			"risk":              string(result.Preview.Risk),
			"requires_approval": result.Preview.RequiresApproval,
			"diff":              result.Diff,
		})
	}
}

func handlePublishLearning(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in publishLearningInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in publishLearningInput) (*mcp.CallToolResult, any, error) {
		pubIn := &publish.PublishInput{
			LearningID:  domain.LearningID(in.LearningID),
			PreviewHash: in.PreviewHash,
			Actor:       toActor(in.Actor),
		}

		svc := publish.NewService(srv.db, srv.projectRoot,
			srv.projectRoot+"/.royo-learn/backups",
			srv.projectRoot+"/.royo-learn")
		result, err := svc.Publish(ctx, srv.projectID, pubIn)
		if err != nil {
			return toolError("publish_failed", err.Error())
		}

		return toolResultJSON(map[string]any{
			"publication_id": string(result.Publication.ID),
			"learning_id":    string(result.Publication.LearningID),
			"status":         string(result.Publication.Status),
			"journal_id":     result.JournalID,
		})
	}
}

func handleListLearnings(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in listLearningsInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in listLearningsInput) (*mcp.CallToolResult, any, error) {
		filter := domain.LearningFilter{
			Limit:  in.Limit,
			Offset: in.Offset,
		}
		if filter.Limit <= 0 || filter.Limit > 100 {
			filter.Limit = 50
		}

		for _, s := range in.Status {
			filter.Status = append(filter.Status, domain.LearningStatus(s))
		}
		for _, t := range in.Type {
			filter.Type = append(filter.Type, domain.LearningType(t))
		}

		tx, err := srv.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return toolError("db_error", err.Error())
		}
		defer tx.Rollback()

		learnings, err := storage.ListLearnings(ctx, tx, srv.projectID, filter)
		if err != nil {
			return toolError("list_failed", err.Error())
		}

		items := make([]map[string]any, 0, len(learnings))
		for _, l := range learnings {
			items = append(items, learningToMap(l))
		}
		return toolResultJSON(items)
	}
}

func handleGetLearning(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in getLearningInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in getLearningInput) (*mcp.CallToolResult, any, error) {
		tx, err := srv.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return toolError("db_error", err.Error())
		}
		defer tx.Rollback()

		learning, err := storage.GetLearning(ctx, tx, domain.LearningID(in.LearningID))
		if err != nil {
			return toolError("get_failed", err.Error())
		}
		if learning == nil {
			return toolError("learning_not_found", fmt.Sprintf("learning %q not found", in.LearningID))
		}

		return toolResultJSON(learningToMap(learning))
	}
}

func handleDoctor(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in doctorInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in doctorInput) (*mcp.CallToolResult, any, error) {
		meta := buildinfo.Current()

		checks := []map[string]any{
			{
				"name":    "database",
				"status":  "pass",
				"message": "database connection ok",
			},
			{
				"name":    "project",
				"status":  "pass",
				"message": "project resolved",
			},
		}
		ok := true

		if err := srv.db.DB.PingContext(ctx); err != nil {
			ok = false
			checks[0] = map[string]any{
				"name":    "database",
				"status":  "fail",
				"message": fmt.Sprintf("database ping failed: %v", err),
			}
		}

		return toolResultJSON(map[string]any{
			"ok":      ok,
			"version": meta.Version,
			"checks":  checks,
		})
	}
}

// ---------------------------------------------------------------------------
// list_recurrences / compute_metrics
// ---------------------------------------------------------------------------

func handleListRecurrences(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in listRecurrencesInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in listRecurrencesInput) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		records, err := recurrence.ListRecurrencesForLearning(ctx, srv.db, domain.LearningID(in.LearningID), limit)
		if err != nil {
			return toolError("list_recurrences_failed", err.Error())
		}

		items := make([]map[string]any, 0, len(records))
		for _, r := range records {
			items = append(items, map[string]any{
				"id":                     string(r.ID),
				"recurrence_fingerprint": r.RecurrenceFingerprint,
				"learning_id":            string(r.LearningID),
				"project_id":             string(r.ProjectID),
				"summary":                r.Summary,
				"occurred_at":            r.OccurredAt.Format(time.RFC3339),
			})
		}
		return toolResultJSON(items)
	}
}

func handleComputeMetrics(srv *Server) func(ctx context.Context, req *mcp.CallToolRequest, in computeMetricsInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in computeMetricsInput) (*mcp.CallToolResult, any, error) {
		tx, err := srv.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return toolError("db_error", err.Error())
		}
		defer tx.Rollback()

		learning, err := storage.GetLearning(ctx, tx, domain.LearningID(in.LearningID))
		if err != nil {
			return toolError("get_learning_failed", err.Error())
		}
		if learning == nil {
			return toolError("learning_not_found", fmt.Sprintf("learning %q not found", in.LearningID))
		}
		tx.Rollback()

		fp := recurrence.RecurrenceFingerprint(learning)
		metrics, err := recurrence.ComputeMetrics(ctx, srv.db, srv.projectID, fp)
		if err != nil {
			return toolError("compute_metrics_failed", err.Error())
		}

		status, _ := recurrence.CheckNeedsReview(ctx, srv.db, srv.projectID, learning)
		metrics.NeedsReview = status.NeedsReview
		if status.Reason != "" {
			metrics.ReviewReason = status.Reason
		}

		return toolResultJSON(map[string]any{
			"fingerprint":   metrics.Fingerprint,
			"count":         metrics.Count,
			"first_seen":    metrics.FirstSeen.Format(time.RFC3339),
			"last_seen":     metrics.LastSeen.Format(time.RFC3339),
			"avg_interval":  metrics.AvgInterval.String(),
			"trend":         string(metrics.Trend),
			"needs_review":  metrics.NeedsReview,
			"review_reason": metrics.ReviewReason,
		})
	}
}

// ---------------------------------------------------------------------------
// learningToMap converts a domain.Learning to a map for JSON output.
// ---------------------------------------------------------------------------

func learningToMap(l *domain.Learning) map[string]any {
	m := map[string]any{
		"id":              string(l.ID),
		"project_id":      string(l.ProjectID),
		"status":          string(l.Status),
		"type":            string(l.Type),
		"title":           l.Title,
		"context":         l.Context,
		"observation":     l.Observation,
		"reusable_lesson": l.ReusableLesson,
		"scope_guess":     string(l.ScopeGuess),
		"confidence":      string(l.Confidence),
		"evidence_level":  string(l.EvidenceLevel),
		"fingerprint":     l.Fingerprint,
		"revision":        l.Revision,
		"created_at":      l.CreatedAt.Format(time.RFC3339),
		"updated_at":      l.UpdatedAt.Format(time.RFC3339),
	}

	if l.ApprovedScope != nil {
		m["approved_scope"] = string(*l.ApprovedScope)
	}
	if l.ApprovedDestination != nil {
		m["approved_destination"] = map[string]any{
			"type": string(l.ApprovedDestination.Type),
			"root": l.ApprovedDestination.Root,
			"path": l.ApprovedDestination.Path,
		}
	}
	if len(l.RecommendedProcedure) > 0 {
		m["recommended_procedure"] = l.RecommendedProcedure
	}
	if len(l.RetrievalTerms) > 0 {
		m["retrieval_terms"] = l.RetrievalTerms
	}

	return m
}
