package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/moderation-service/internal/models"
	"github.com/google/uuid"
)

type fakeModerationRepo struct {
	blocks  map[uuid.UUID]map[uuid.UUID]bool
	mutes   map[uuid.UUID]map[uuid.UUID]bool
	reports []models.Report

	visibility models.Visibility
	created    *models.Report
	duplicate  bool
	repoErr    error

	resolvedID     uuid.UUID
	resolvedStatus string
	lastListStatus string
	lastListLimit  int
	lastListCursor models.ReportCursor
}

func newFakeModerationRepo() *fakeModerationRepo {
	return &fakeModerationRepo{
		blocks: make(map[uuid.UUID]map[uuid.UUID]bool),
		mutes:  make(map[uuid.UUID]map[uuid.UUID]bool),
	}
}

func (r *fakeModerationRepo) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if r.repoErr != nil {
		return r.repoErr
	}
	if r.blocks[blockerID] == nil {
		r.blocks[blockerID] = make(map[uuid.UUID]bool)
	}
	r.blocks[blockerID][blockedID] = true
	return nil
}

func (r *fakeModerationRepo) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if r.repoErr != nil {
		return r.repoErr
	}
	delete(r.blocks[blockerID], blockedID)
	return nil
}

func (r *fakeModerationRepo) Mute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	if r.repoErr != nil {
		return r.repoErr
	}
	if r.mutes[muterID] == nil {
		r.mutes[muterID] = make(map[uuid.UUID]bool)
	}
	r.mutes[muterID][mutedID] = true
	return nil
}

func (r *fakeModerationRepo) Unmute(ctx context.Context, muterID, mutedID uuid.UUID) error {
	if r.repoErr != nil {
		return r.repoErr
	}
	delete(r.mutes[muterID], mutedID)
	return nil
}

func (r *fakeModerationRepo) ListBlocked(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if r.repoErr != nil {
		return nil, r.repoErr
	}
	ids := make([]uuid.UUID, 0, len(r.blocks[userID]))
	for id := range r.blocks[userID] {
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *fakeModerationRepo) ListMuted(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if r.repoErr != nil {
		return nil, r.repoErr
	}
	ids := make([]uuid.UUID, 0, len(r.mutes[userID]))
	for id := range r.mutes[userID] {
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *fakeModerationRepo) GetVisibility(ctx context.Context, viewerID, targetID uuid.UUID) (models.Visibility, error) {
	if r.repoErr != nil {
		return models.Visibility{}, r.repoErr
	}
	return r.visibility, nil
}

func (r *fakeModerationRepo) CreateReport(ctx context.Context, report *models.Report) (*models.Report, bool, error) {
	if r.repoErr != nil {
		return nil, false, r.repoErr
	}
	if r.created != nil {
		return r.created, r.duplicate, nil
	}
	stored := *report
	stored.ID = uuid.New()
	stored.Status = models.StatusPending
	stored.CreatedAt = time.Now().UTC()
	return &stored, r.duplicate, nil
}

func (r *fakeModerationRepo) ListReports(ctx context.Context, status string, cursor models.ReportCursor, limit int) ([]models.Report, error) {
	if r.repoErr != nil {
		return nil, r.repoErr
	}
	r.lastListStatus = status
	r.lastListLimit = limit
	r.lastListCursor = cursor

	reports := make([]models.Report, 0, limit+1)
	base := time.Now().UTC()
	for i := 0; i <= limit; i++ {
		reports = append(reports, models.Report{
			ID:        uuid.New(),
			Status:    models.StatusPending,
			CreatedAt: base.Add(time.Duration(-i) * time.Second),
		})
	}
	return reports, nil
}

func (r *fakeModerationRepo) ResolveReport(ctx context.Context, reportID, reviewerID uuid.UUID, status, note string) (*models.Report, error) {
	if r.repoErr != nil {
		return nil, r.repoErr
	}
	r.resolvedID = reportID
	r.resolvedStatus = status
	now := time.Now().UTC()
	return &models.Report{
		ID:         reportID,
		Status:     status,
		ReviewerID: &reviewerID,
		ReviewedAt: &now,
	}, nil
}

func TestBlockUserRejectsSelfAndInvalidIDs(t *testing.T) {
	service := NewModerationService(newFakeModerationRepo())
	id := uuid.New()

	if err := service.BlockUser(context.Background(), id.String(), id.String()); !errors.Is(err, models.ErrCannotModerateSelf) {
		t.Fatalf("self block err = %v, want ErrCannotModerateSelf", err)
	}
	if err := service.BlockUser(context.Background(), "nope", id.String()); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad blocker err = %v, want ErrInvalidInput", err)
	}
	if err := service.MuteUser(context.Background(), id.String(), "nope"); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad muted err = %v, want ErrInvalidInput", err)
	}
}

func TestRelationshipMutationsAreScopedAndReversible(t *testing.T) {
	repo := newFakeModerationRepo()
	service := NewModerationService(repo)
	alice, bob := uuid.New(), uuid.New()

	if err := service.BlockUser(context.Background(), alice.String(), bob.String()); err != nil {
		t.Fatalf("block: %v", err)
	}
	blocked, err := service.ListBlocked(context.Background(), alice.String())
	if err != nil {
		t.Fatalf("list blocked: %v", err)
	}
	if len(blocked) != 1 || blocked[0] != bob.String() {
		t.Fatalf("blocked = %v, want [%s]", blocked, bob)
	}

	if err := service.UnblockUser(context.Background(), alice.String(), bob.String()); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if err := service.MuteUser(context.Background(), alice.String(), bob.String()); err != nil {
		t.Fatalf("mute: %v", err)
	}
	muted, err := service.ListMuted(context.Background(), alice.String())
	if err != nil {
		t.Fatalf("list muted: %v", err)
	}
	if len(muted) != 1 || muted[0] != bob.String() {
		t.Fatalf("muted = %v, want [%s]", muted, bob)
	}
}

func TestGetStatusReturnsEmptyForSelf(t *testing.T) {
	repo := newFakeModerationRepo()
	repo.visibility = models.Visibility{IsBlocked: true}
	service := NewModerationService(repo)
	id := uuid.New()

	visibility, err := service.GetStatus(context.Background(), id.String(), id.String())
	if err != nil {
		t.Fatalf("self status: %v", err)
	}
	if visibility.IsBlocked || visibility.HasBlockedViewer || visibility.IsMuted {
		t.Fatalf("self visibility = %+v, want empty", visibility)
	}
}

func TestReportContentValidatesEnumsAndDetails(t *testing.T) {
	service := NewModerationService(newFakeModerationRepo())
	reporter, target := uuid.New(), uuid.New()

	cases := map[string]struct {
		reporter   string
		targetType string
		targetID   string
		reason     string
		details    string
		want       error
	}{
		"bad reporter": {"nope", models.TargetPost, target.String(), models.ReasonSpam, "", models.ErrInvalidInput},
		"bad target":   {reporter.String(), models.TargetPost, "nope", models.ReasonSpam, "", models.ErrInvalidInput},
		"bad type":     {reporter.String(), "MESSAGE", target.String(), models.ReasonSpam, "", models.ErrInvalidInput},
		"bad reason":   {reporter.String(), models.TargetPost, target.String(), "BORING", "", models.ErrInvalidInput},
		"long details": {reporter.String(), models.TargetPost, target.String(), models.ReasonSpam, strings.Repeat("a", maxReportDetails+1), models.ErrInvalidInput},
		"report self":  {reporter.String(), models.TargetUser, reporter.String(), models.ReasonSpam, "", models.ErrCannotModerateSelf},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := service.ReportContent(context.Background(), tc.reporter, tc.targetType, tc.targetID, tc.reason, tc.details)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestReportContentNormalisesInput(t *testing.T) {
	repo := newFakeModerationRepo()
	service := NewModerationService(repo)
	reporter, target := uuid.New(), uuid.New()

	report, duplicate, err := service.ReportContent(
		context.Background(),
		reporter.String(),
		" user ",
		target.String(),
		" harassment ",
		"  abusive reply  ",
	)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if duplicate {
		t.Fatal("first report should not be a duplicate")
	}
	if report.TargetType != models.TargetUser || report.Reason != models.ReasonHarassment {
		t.Fatalf("report = %+v", report)
	}
	if report.Details != "abusive reply" {
		t.Fatalf("details = %q, want trimmed", report.Details)
	}
}

func TestListReportsClampsAndPaginates(t *testing.T) {
	repo := newFakeModerationRepo()
	service := NewModerationService(repo)

	page, err := service.ListReports(context.Background(), "", 1000, "")
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if repo.lastListLimit != maxPageSize {
		t.Fatalf("limit = %d, want %d", repo.lastListLimit, maxPageSize)
	}
	if len(page.Reports) != maxPageSize || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("page = %d reports, has_more=%v, cursor=%q", len(page.Reports), page.HasMore, page.NextCursor)
	}

	if _, err := service.ListReports(context.Background(), "NOPE", 0, ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad status err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.ListReports(context.Background(), "", 0, "not-a-cursor"); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad cursor err = %v, want ErrInvalidInput", err)
	}
}

func TestResolveReportValidatesStatusAndID(t *testing.T) {
	repo := newFakeModerationRepo()
	service := NewModerationService(repo)
	reportID, reviewer := uuid.New(), uuid.New()

	if _, err := service.ResolveReport(context.Background(), "nope", reviewer.String(), models.StatusActioned, ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("bad report id err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.ResolveReport(context.Background(), reportID.String(), reviewer.String(), models.StatusPending, ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("pending status err = %v, want ErrInvalidInput", err)
	}
	if _, err := service.ResolveReport(context.Background(), reportID.String(), reviewer.String(), models.StatusActioned, strings.Repeat("a", maxReportDetails+1)); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("long note err = %v, want ErrInvalidInput", err)
	}

	report, err := service.ResolveReport(context.Background(), reportID.String(), reviewer.String(), " dismissed ", " no action ")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if repo.resolvedID != reportID || repo.resolvedStatus != models.StatusDismissed {
		t.Fatalf("resolved = %s/%s", repo.resolvedID, repo.resolvedStatus)
	}
	if report.Status != models.StatusDismissed {
		t.Fatalf("report status = %q", report.Status)
	}
}

func TestResolveReportPreservesClosedError(t *testing.T) {
	repo := newFakeModerationRepo()
	repo.repoErr = models.ErrReportClosed
	service := NewModerationService(repo)

	_, err := service.ResolveReport(context.Background(), uuid.New().String(), uuid.New().String(), models.StatusDismissed, "")
	if !errors.Is(err, models.ErrReportClosed) {
		t.Fatalf("err = %v, want ErrReportClosed", err)
	}
}
