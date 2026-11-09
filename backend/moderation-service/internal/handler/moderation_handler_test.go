package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chetansachan/socialflow/moderation-service/internal/models"
	moderationpb "github.com/chetansachan/socialflow/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeModerationService struct {
	err error

	visibility models.Visibility
	report     *models.Report
	duplicate  bool
	page       *models.ReportPage

	blockerID    string
	blockedID    string
	reporterID   string
	targetType   string
	targetID     string
	reason       string
	listStatus   string
	listCursor   string
	listLimit    int32
	resolvedTo   string
	resolvedNote string
}

func (s *fakeModerationService) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	s.blockerID, s.blockedID = blockerID, blockedID
	return s.err
}

func (s *fakeModerationService) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	s.blockerID, s.blockedID = blockerID, blockedID
	return s.err
}

func (s *fakeModerationService) MuteUser(ctx context.Context, muterID, mutedID string) error {
	s.blockerID, s.blockedID = muterID, mutedID
	return s.err
}

func (s *fakeModerationService) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	s.blockerID, s.blockedID = muterID, mutedID
	return s.err
}

func (s *fakeModerationService) GetStatus(ctx context.Context, viewerID, targetID string) (models.Visibility, error) {
	s.blockerID, s.blockedID = viewerID, targetID
	return s.visibility, s.err
}

func (s *fakeModerationService) ListBlocked(ctx context.Context, userID string) ([]string, error) {
	return []string{"u1"}, s.err
}

func (s *fakeModerationService) ListMuted(ctx context.Context, userID string) ([]string, error) {
	return []string{"u2"}, s.err
}

func (s *fakeModerationService) ReportContent(ctx context.Context, reporterID, targetType, targetID, reason, details string) (*models.Report, bool, error) {
	s.reporterID, s.targetType, s.targetID, s.reason = reporterID, targetType, targetID, reason
	return s.report, s.duplicate, s.err
}

func (s *fakeModerationService) ListReports(ctx context.Context, status string, limit int32, cursor string) (*models.ReportPage, error) {
	s.listStatus, s.listLimit, s.listCursor = status, limit, cursor
	return s.page, s.err
}

func (s *fakeModerationService) ResolveReport(ctx context.Context, reportID, reviewerID, status, note string) (*models.Report, error) {
	s.resolvedTo, s.resolvedNote = status, note
	return s.report, s.err
}

func sampleReport() *models.Report {
	now := time.Now().UTC().Round(0)
	reviewer := uuid.New()
	return &models.Report{
		ID:         uuid.New(),
		ReporterID: uuid.New(),
		TargetType: models.TargetComment,
		TargetID:   uuid.New(),
		Reason:     models.ReasonHarassment,
		Details:    "abusive",
		Status:     models.StatusActioned,
		CreatedAt:  now,
		ReviewedAt: &now,
		ReviewerID: &reviewer,
		ReviewNote: "removed",
	}
}

func TestHandlerScopesBlocksToRequestIDs(t *testing.T) {
	service := &fakeModerationService{}
	h := NewModerationHandler(service)

	blocker, blocked := uuid.New().String(), uuid.New().String()
	if _, err := h.BlockUser(context.Background(), &moderationpb.BlockUserRequest{
		BlockerId: blocker,
		BlockedId: blocked,
	}); err != nil {
		t.Fatalf("block: %v", err)
	}
	if service.blockerID != blocker || service.blockedID != blocked {
		t.Fatalf("service received %s/%s", service.blockerID, service.blockedID)
	}

	if _, err := h.UnblockUser(context.Background(), &moderationpb.BlockUserRequest{
		BlockerId: blocker,
		BlockedId: blocked,
	}); err != nil {
		t.Fatalf("unblock: %v", err)
	}

	res, err := h.GetModerationStatus(context.Background(), &moderationpb.GetModerationStatusRequest{
		ViewerId: blocker,
		TargetId: blocked,
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if res.GetIsBlocked() || res.GetHasBlockedViewer() || res.GetIsMuted() {
		t.Fatalf("status = %+v", res)
	}
}

func TestHandlerMapsReportEnums(t *testing.T) {
	service := &fakeModerationService{report: sampleReport(), duplicate: true}
	h := NewModerationHandler(service)

	res, err := h.ReportContent(context.Background(), &moderationpb.ReportContentRequest{
		ReporterId: uuid.New().String(),
		TargetType: moderationpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT,
		TargetId:   uuid.New().String(),
		Reason:     moderationpb.ReportReason_REPORT_REASON_HARASSMENT,
		Details:    "abusive",
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if service.targetType != models.TargetComment || service.reason != models.ReasonHarassment {
		t.Fatalf("service received %s/%s", service.targetType, service.reason)
	}
	if !res.GetDuplicate() {
		t.Fatal("duplicate flag was lost")
	}

	report := res.GetReport()
	if report.GetTargetType() != moderationpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT {
		t.Fatalf("target type = %v", report.GetTargetType())
	}
	if report.GetReason() != moderationpb.ReportReason_REPORT_REASON_HARASSMENT {
		t.Fatalf("reason = %v", report.GetReason())
	}
	if report.GetStatus() != moderationpb.ReportStatus_REPORT_STATUS_ACTIONED {
		t.Fatalf("status = %v", report.GetStatus())
	}
	if report.GetReviewedAt() == "" || report.GetReviewerId() == "" || report.GetReviewNote() != "removed" {
		t.Fatalf("review metadata = %+v", report)
	}
}

func TestHandlerUnknownEnumsBecomeInvalidInput(t *testing.T) {
	service := &fakeModerationService{report: sampleReport()}
	h := NewModerationHandler(service)

	_, err := h.ReportContent(context.Background(), &moderationpb.ReportContentRequest{
		ReporterId: uuid.New().String(),
		TargetType: moderationpb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED,
		TargetId:   uuid.New().String(),
		Reason:     moderationpb.ReportReason_REPORT_REASON_UNSPECIFIED,
	})
	if err != nil {
		t.Fatalf("handler should pass empty enums to the service: %v", err)
	}
	if service.targetType != "" || service.reason != "" {
		t.Fatalf("service received %q/%q, want empty strings", service.targetType, service.reason)
	}
}

func TestHandlerListReportsPassesFiltersAndMapsPage(t *testing.T) {
	service := &fakeModerationService{page: &models.ReportPage{
		Reports:    []models.Report{*sampleReport()},
		NextCursor: "next",
		HasMore:    true,
	}}
	h := NewModerationHandler(service)

	res, err := h.ListReports(context.Background(), &moderationpb.ListReportsRequest{
		Status: moderationpb.ReportStatus_REPORT_STATUS_DISMISSED,
		Limit:  25,
		Cursor: "opaque",
	})
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if service.listStatus != models.StatusDismissed || service.listLimit != 25 || service.listCursor != "opaque" {
		t.Fatalf("service received %s/%d/%s", service.listStatus, service.listLimit, service.listCursor)
	}
	if len(res.GetReports()) != 1 || res.GetNextCursor() != "next" || !res.GetHasMore() {
		t.Fatalf("response = %+v", res)
	}
}

func TestHandlerMapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{models.ErrInvalidInput, codes.InvalidArgument},
		{models.ErrCannotModerateSelf, codes.InvalidArgument},
		{models.ErrReportNotFound, codes.NotFound},
		{models.ErrReportClosed, codes.FailedPrecondition},
		{errors.New("connection reset"), codes.Internal},
	}
	for _, tc := range cases {
		h := NewModerationHandler(&fakeModerationService{err: tc.err})

		_, err := h.BlockUser(context.Background(), &moderationpb.BlockUserRequest{
			BlockerId: uuid.New().String(),
			BlockedId: uuid.New().String(),
		})
		if status.Code(err) != tc.want {
			t.Fatalf("%v: code = %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}
