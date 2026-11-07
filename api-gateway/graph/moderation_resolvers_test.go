package graph

import (
	"context"
	"testing"

	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/chetansachan/socialflow/api-gateway/graph/model"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
)

type fakeModerationClient struct {
	socialflowpb.ModerationServiceClient

	blockRequest   *socialflowpb.BlockUserRequest
	muteRequest    *socialflowpb.MuteUserRequest
	statusRequest  *socialflowpb.GetModerationStatusRequest
	listRequest    *socialflowpb.ListRelationshipsRequest
	reportRequest  *socialflowpb.ReportContentRequest
	resolveRequest *socialflowpb.ResolveReportRequest
	reportsRequest *socialflowpb.ListReportsRequest

	message    string
	visibility *socialflowpb.GetModerationStatusResponse
	userIDs    []string
	report     *socialflowpb.Report
	duplicate  bool
	reports    []*socialflowpb.Report
	nextCursor string
	hasMore    bool
}

func (c *fakeModerationClient) BlockUser(ctx context.Context, req *socialflowpb.BlockUserRequest, _ ...grpc.CallOption) (*socialflowpb.ModerationActionResponse, error) {
	c.blockRequest = req
	return &socialflowpb.ModerationActionResponse{Message: c.message}, nil
}

func (c *fakeModerationClient) UnblockUser(ctx context.Context, req *socialflowpb.BlockUserRequest, _ ...grpc.CallOption) (*socialflowpb.ModerationActionResponse, error) {
	c.blockRequest = req
	return &socialflowpb.ModerationActionResponse{Message: c.message}, nil
}

func (c *fakeModerationClient) MuteUser(ctx context.Context, req *socialflowpb.MuteUserRequest, _ ...grpc.CallOption) (*socialflowpb.ModerationActionResponse, error) {
	c.muteRequest = req
	return &socialflowpb.ModerationActionResponse{Message: c.message}, nil
}

func (c *fakeModerationClient) UnmuteUser(ctx context.Context, req *socialflowpb.MuteUserRequest, _ ...grpc.CallOption) (*socialflowpb.ModerationActionResponse, error) {
	c.muteRequest = req
	return &socialflowpb.ModerationActionResponse{Message: c.message}, nil
}

func (c *fakeModerationClient) GetModerationStatus(ctx context.Context, req *socialflowpb.GetModerationStatusRequest, _ ...grpc.CallOption) (*socialflowpb.GetModerationStatusResponse, error) {
	c.statusRequest = req
	if c.visibility == nil {
		return &socialflowpb.GetModerationStatusResponse{}, nil
	}
	return c.visibility, nil
}

func (c *fakeModerationClient) ListBlockedUsers(ctx context.Context, req *socialflowpb.ListRelationshipsRequest, _ ...grpc.CallOption) (*socialflowpb.ListRelationshipsResponse, error) {
	c.listRequest = req
	return &socialflowpb.ListRelationshipsResponse{UserIds: c.userIDs}, nil
}

func (c *fakeModerationClient) ListMutedUsers(ctx context.Context, req *socialflowpb.ListRelationshipsRequest, _ ...grpc.CallOption) (*socialflowpb.ListRelationshipsResponse, error) {
	c.listRequest = req
	return &socialflowpb.ListRelationshipsResponse{UserIds: c.userIDs}, nil
}

func (c *fakeModerationClient) ReportContent(ctx context.Context, req *socialflowpb.ReportContentRequest, _ ...grpc.CallOption) (*socialflowpb.ReportContentResponse, error) {
	c.reportRequest = req
	return &socialflowpb.ReportContentResponse{Report: c.report, Duplicate: c.duplicate}, nil
}

func (c *fakeModerationClient) ListReports(ctx context.Context, req *socialflowpb.ListReportsRequest, _ ...grpc.CallOption) (*socialflowpb.ListReportsResponse, error) {
	c.reportsRequest = req
	return &socialflowpb.ListReportsResponse{
		Reports:    c.reports,
		NextCursor: c.nextCursor,
		HasMore:    c.hasMore,
	}, nil
}

func (c *fakeModerationClient) ResolveReport(ctx context.Context, req *socialflowpb.ResolveReportRequest, _ ...grpc.CallOption) (*socialflowpb.ResolveReportResponse, error) {
	c.resolveRequest = req
	return &socialflowpb.ResolveReportResponse{Report: c.report}, nil
}

func adminContext(userID string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{UserID: userID, Role: "admin"})
}

func sampleProtoReport() *socialflowpb.Report {
	return &socialflowpb.Report{
		Id:         "r1",
		ReporterId: "u1",
		TargetType: socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT,
		TargetId:   "c1",
		Reason:     socialflowpb.ReportReason_REPORT_REASON_HARASSMENT,
		Details:    "abusive reply",
		Status:     socialflowpb.ReportStatus_REPORT_STATUS_ACTIONED,
		CreatedAt:  "2026-01-01T00:00:00Z",
		ReviewedAt: "2026-01-02T00:00:00Z",
		ReviewerId: "mod1",
		ReviewNote: "removed",
	}
}

func TestModerationMutationsScopeActorToPrincipal(t *testing.T) {
	client := &fakeModerationClient{message: "user blocked"}
	resolver := &mutationResolver{&Resolver{ModerationClient: client}}

	res, err := resolver.BlockUser(viewerContext("u1"), "u2")
	if err != nil {
		t.Fatalf("BlockUser: %v", err)
	}
	if client.blockRequest.GetBlockerId() != "u1" || client.blockRequest.GetBlockedId() != "u2" {
		t.Fatalf("block request = %+v", client.blockRequest)
	}
	if res.Message != "user blocked" {
		t.Fatalf("message = %q", res.Message)
	}

	if _, err := resolver.MuteUser(viewerContext("u1"), "u3"); err != nil {
		t.Fatalf("MuteUser: %v", err)
	}
	if client.muteRequest.GetMuterId() != "u1" || client.muteRequest.GetMutedId() != "u3" {
		t.Fatalf("mute request = %+v", client.muteRequest)
	}
}

func TestModerationMutationsRequireAuthentication(t *testing.T) {
	resolver := &mutationResolver{&Resolver{ModerationClient: &fakeModerationClient{}}}
	ctx := context.Background()

	if _, err := resolver.BlockUser(ctx, "u2"); err == nil {
		t.Fatal("anonymous block should fail")
	}
	if _, err := resolver.UnblockUser(ctx, "u2"); err == nil {
		t.Fatal("anonymous unblock should fail")
	}
	if _, err := resolver.MuteUser(ctx, "u2"); err == nil {
		t.Fatal("anonymous mute should fail")
	}
	if _, err := resolver.UnmuteUser(ctx, "u2"); err == nil {
		t.Fatal("anonymous unmute should fail")
	}
	if _, err := resolver.ReportContent(ctx, model.ReportContentInput{
		TargetType: model.ReportTargetTypePost,
		TargetID:   "p1",
		Reason:     model.ReportReasonSpam,
	}); err == nil {
		t.Fatal("anonymous report should fail")
	}
	if _, err := resolver.ResolveReport(ctx, "r1", model.ReportStatusDismissed, nil); err == nil {
		t.Fatal("anonymous resolve should fail")
	}
}

func TestReportContentMapsEnumsAndScopesReporter(t *testing.T) {
	client := &fakeModerationClient{report: sampleProtoReport(), duplicate: true}
	resolver := &mutationResolver{&Resolver{ModerationClient: client}}

	details := "abusive reply"
	res, err := resolver.ReportContent(viewerContext("u1"), model.ReportContentInput{
		TargetType: model.ReportTargetTypeComment,
		TargetID:   "c1",
		Reason:     model.ReportReasonHarassment,
		Details:    &details,
	})
	if err != nil {
		t.Fatalf("ReportContent: %v", err)
	}

	request := client.reportRequest
	if request.GetReporterId() != "u1" {
		t.Fatalf("reporter = %q, want the authenticated caller", request.GetReporterId())
	}
	if request.GetTargetType() != socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT {
		t.Fatalf("target type = %v", request.GetTargetType())
	}
	if request.GetReason() != socialflowpb.ReportReason_REPORT_REASON_HARASSMENT {
		t.Fatalf("reason = %v", request.GetReason())
	}
	if request.GetDetails() != details {
		t.Fatalf("details = %q", request.GetDetails())
	}
	if !res.Duplicate {
		t.Fatal("duplicate flag was lost")
	}
	if res.Report == nil || res.Report.TargetType != model.ReportTargetTypeComment {
		t.Fatalf("report = %+v", res.Report)
	}
}

func TestToModelReportMapsOptionalReviewMetadata(t *testing.T) {
	mapped := toModelReport(sampleProtoReport())
	if mapped == nil {
		t.Fatal("nil mapping")
	}
	if mapped.Status != model.ReportStatusActioned || mapped.Reason != model.ReportReasonHarassment {
		t.Fatalf("report = %+v", mapped)
	}
	if mapped.ReviewedAt == nil || *mapped.ReviewedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("reviewed at = %v", mapped.ReviewedAt)
	}
	if mapped.ReviewerID == nil || *mapped.ReviewerID != "mod1" {
		t.Fatalf("reviewer = %v", mapped.ReviewerID)
	}

	open := sampleProtoReport()
	open.ReviewedAt = ""
	open.ReviewerId = ""
	open.ReviewNote = ""
	mapped = toModelReport(open)
	if mapped.ReviewedAt != nil || mapped.ReviewerID != nil {
		t.Fatalf("open report leaked review metadata: %+v", mapped)
	}
	if toModelReport(nil) != nil {
		t.Fatal("nil report should map to nil")
	}
}

func TestGetReportsRequiresAdmin(t *testing.T) {
	client := &fakeModerationClient{}
	resolver := &queryResolver{&Resolver{ModerationClient: client}}

	limit := int32(10)
	if _, err := resolver.GetReports(viewerContext("u1"), nil, &limit, nil); err == nil {
		t.Fatal("non-admin should not read the moderation queue")
	}
	if client.reportsRequest != nil {
		t.Fatal("the queue was queried before the role check")
	}

	if _, err := resolver.GetReports(context.Background(), nil, &limit, nil); err == nil {
		t.Fatal("anonymous caller should not read the moderation queue")
	}
}

func TestGetReportsClampsPageAndMapsReports(t *testing.T) {
	client := &fakeModerationClient{
		reports:    []*socialflowpb.Report{sampleProtoReport()},
		nextCursor: "cursor-2",
		hasMore:    true,
	}
	resolver := &queryResolver{&Resolver{ModerationClient: client}}

	status := model.ReportStatusDismissed
	limit := int32(1000)
	res, err := resolver.GetReports(adminContext("mod1"), &status, &limit, nil)
	if err != nil {
		t.Fatalf("GetReports: %v", err)
	}

	if client.reportsRequest.GetLimit() != maxReportPageSize {
		t.Fatalf("limit = %d, want %d", client.reportsRequest.GetLimit(), maxReportPageSize)
	}
	if client.reportsRequest.GetStatus() != socialflowpb.ReportStatus_REPORT_STATUS_DISMISSED {
		t.Fatalf("status = %v", client.reportsRequest.GetStatus())
	}
	if len(res.Reports) != 1 || res.Reports[0].ID != "r1" {
		t.Fatalf("reports = %+v", res.Reports)
	}
	if res.NextCursor == nil || *res.NextCursor != "cursor-2" || !res.HasMore {
		t.Fatalf("page metadata = %+v", res)
	}
}

func TestGetReportsDefaultsToOpenQueue(t *testing.T) {
	client := &fakeModerationClient{}
	resolver := &queryResolver{&Resolver{ModerationClient: client}}

	if _, err := resolver.GetReports(adminContext("mod1"), nil, nil, nil); err != nil {
		t.Fatalf("GetReports: %v", err)
	}
	if client.reportsRequest.GetStatus() != socialflowpb.ReportStatus_REPORT_STATUS_UNSPECIFIED {
		t.Fatalf("status = %v, want unspecified so the service returns open reports", client.reportsRequest.GetStatus())
	}
	if client.reportsRequest.GetLimit() != defaultReportPageSize {
		t.Fatalf("limit = %d, want %d", client.reportsRequest.GetLimit(), defaultReportPageSize)
	}

	empty := model.ReportStatus("")
	if _, err := resolver.GetReports(adminContext("mod1"), &empty, nil, nil); err != nil {
		t.Fatalf("GetReports with empty status: %v", err)
	}
	if client.reportsRequest.GetStatus() != socialflowpb.ReportStatus_REPORT_STATUS_UNSPECIFIED {
		t.Fatalf("empty status = %v, want unspecified", client.reportsRequest.GetStatus())
	}
}

func TestResolveReportRequiresAdminAndScopesReviewer(t *testing.T) {
	client := &fakeModerationClient{report: sampleProtoReport()}
	resolver := &mutationResolver{&Resolver{ModerationClient: client}}

	if _, err := resolver.ResolveReport(viewerContext("u1"), "r1", model.ReportStatusDismissed, nil); err == nil {
		t.Fatal("non-admin should not resolve reports")
	}
	if client.resolveRequest != nil {
		t.Fatal("the report was resolved before the role check")
	}

	note := "removed"
	res, err := resolver.ResolveReport(adminContext("mod1"), "r1", model.ReportStatusDismissed, &note)
	if err != nil {
		t.Fatalf("ResolveReport: %v", err)
	}
	if client.resolveRequest.GetReviewerId() != "mod1" || client.resolveRequest.GetReportId() != "r1" {
		t.Fatalf("resolve request = %+v", client.resolveRequest)
	}
	if client.resolveRequest.GetStatus() != socialflowpb.ReportStatus_REPORT_STATUS_DISMISSED {
		t.Fatalf("status = %v", client.resolveRequest.GetStatus())
	}
	if client.resolveRequest.GetNote() != note {
		t.Fatalf("note = %q", client.resolveRequest.GetNote())
	}
	if res == nil || res.Status != model.ReportStatusActioned {
		t.Fatalf("resolved report = %+v", res)
	}
}

func TestModerationStatusAndListsScopeViewer(t *testing.T) {
	client := &fakeModerationClient{
		visibility: &socialflowpb.GetModerationStatusResponse{
			IsBlocked:        true,
			HasBlockedViewer: false,
			IsMuted:          true,
		},
		userIDs: []string{"u2", "u3"},
	}
	resolver := &queryResolver{&Resolver{ModerationClient: client}}

	status, err := resolver.GetModerationStatus(viewerContext("u1"), "u2")
	if err != nil {
		t.Fatalf("GetModerationStatus: %v", err)
	}
	if client.statusRequest.GetViewerId() != "u1" || client.statusRequest.GetTargetId() != "u2" {
		t.Fatalf("status request = %+v", client.statusRequest)
	}
	if !status.IsBlocked || status.HasBlockedViewer || !status.IsMuted {
		t.Fatalf("status = %+v", status)
	}

	blocked, err := resolver.GetBlockedUsers(viewerContext("u1"))
	if err != nil {
		t.Fatalf("GetBlockedUsers: %v", err)
	}
	if client.listRequest.GetUserId() != "u1" {
		t.Fatalf("list request = %+v", client.listRequest)
	}
	if len(blocked) != 2 || blocked[0] != "u2" {
		t.Fatalf("blocked = %v", blocked)
	}

	muted, err := resolver.GetMutedUsers(viewerContext("u1"))
	if err != nil {
		t.Fatalf("GetMutedUsers: %v", err)
	}
	if len(muted) != 2 {
		t.Fatalf("muted = %v", muted)
	}

	if _, err := resolver.GetModerationStatus(context.Background(), "u2"); err == nil {
		t.Fatal("anonymous moderation status should fail")
	}
	if _, err := resolver.GetBlockedUsers(context.Background()); err == nil {
		t.Fatal("anonymous block list should fail")
	}
	if _, err := resolver.GetMutedUsers(context.Background()); err == nil {
		t.Fatal("anonymous mute list should fail")
	}
}

func TestReportEnumMappersRoundTrip(t *testing.T) {
	if got := reportTargetTypeToProto(model.ReportTargetTypeUser); got != socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_USER {
		t.Fatalf("target type = %v", got)
	}
	if got := reportTargetTypeFromProto(socialflowpb.ReportTargetType_REPORT_TARGET_TYPE_POST); got != model.ReportTargetTypePost {
		t.Fatalf("target type = %v", got)
	}
	if got := reportReasonToProto(model.ReportReasonSelfHarm); got != socialflowpb.ReportReason_REPORT_REASON_SELF_HARM {
		t.Fatalf("reason = %v", got)
	}
	if got := reportReasonFromProto(socialflowpb.ReportReason_REPORT_REASON_OTHER); got != model.ReportReasonOther {
		t.Fatalf("reason = %v", got)
	}
	if got := reportStatusToProto(model.ReportStatusReviewing); got != socialflowpb.ReportStatus_REPORT_STATUS_REVIEWING {
		t.Fatalf("status = %v", got)
	}
	if got := reportStatusFromProto(socialflowpb.ReportStatus_REPORT_STATUS_PENDING); got != model.ReportStatusPending {
		t.Fatalf("status = %v", got)
	}
}
