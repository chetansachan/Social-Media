package handler

import (
	"context"
	"errors"
	"time"

	"github.com/chetansachan/socialflow/moderation-service/internal/models"
	"github.com/chetansachan/socialflow/moderation-service/internal/services"
	moderationpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ModerationHandler struct {
	moderationpb.UnimplementedModerationServiceServer
	moderationService services.ModerationService
}

func NewModerationHandler(moderationService services.ModerationService) *ModerationHandler {
	return &ModerationHandler{moderationService: moderationService}
}

func moderationStatus(err error) error {
	switch {
	case errors.Is(err, models.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, models.ErrCannotModerateSelf):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, models.ErrReportNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, models.ErrReportClosed):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func targetTypeFromProto(value moderationpb.ReportTargetType) string {
	switch value {
	case moderationpb.ReportTargetType_REPORT_TARGET_TYPE_POST:
		return models.TargetPost
	case moderationpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT:
		return models.TargetComment
	case moderationpb.ReportTargetType_REPORT_TARGET_TYPE_USER:
		return models.TargetUser
	default:
		return ""
	}
}

func targetTypeToProto(value string) moderationpb.ReportTargetType {
	switch value {
	case models.TargetPost:
		return moderationpb.ReportTargetType_REPORT_TARGET_TYPE_POST
	case models.TargetComment:
		return moderationpb.ReportTargetType_REPORT_TARGET_TYPE_COMMENT
	case models.TargetUser:
		return moderationpb.ReportTargetType_REPORT_TARGET_TYPE_USER
	default:
		return moderationpb.ReportTargetType_REPORT_TARGET_TYPE_UNSPECIFIED
	}
}

func reasonFromProto(value moderationpb.ReportReason) string {
	switch value {
	case moderationpb.ReportReason_REPORT_REASON_SPAM:
		return models.ReasonSpam
	case moderationpb.ReportReason_REPORT_REASON_HARASSMENT:
		return models.ReasonHarassment
	case moderationpb.ReportReason_REPORT_REASON_HATE:
		return models.ReasonHate
	case moderationpb.ReportReason_REPORT_REASON_VIOLENCE:
		return models.ReasonViolence
	case moderationpb.ReportReason_REPORT_REASON_NSFW:
		return models.ReasonNSFW
	case moderationpb.ReportReason_REPORT_REASON_IMPERSONATION:
		return models.ReasonImpersonation
	case moderationpb.ReportReason_REPORT_REASON_SELF_HARM:
		return models.ReasonSelfHarm
	case moderationpb.ReportReason_REPORT_REASON_OTHER:
		return models.ReasonOther
	default:
		return ""
	}
}

func reasonToProto(value string) moderationpb.ReportReason {
	switch value {
	case models.ReasonSpam:
		return moderationpb.ReportReason_REPORT_REASON_SPAM
	case models.ReasonHarassment:
		return moderationpb.ReportReason_REPORT_REASON_HARASSMENT
	case models.ReasonHate:
		return moderationpb.ReportReason_REPORT_REASON_HATE
	case models.ReasonViolence:
		return moderationpb.ReportReason_REPORT_REASON_VIOLENCE
	case models.ReasonNSFW:
		return moderationpb.ReportReason_REPORT_REASON_NSFW
	case models.ReasonImpersonation:
		return moderationpb.ReportReason_REPORT_REASON_IMPERSONATION
	case models.ReasonSelfHarm:
		return moderationpb.ReportReason_REPORT_REASON_SELF_HARM
	case models.ReasonOther:
		return moderationpb.ReportReason_REPORT_REASON_OTHER
	default:
		return moderationpb.ReportReason_REPORT_REASON_UNSPECIFIED
	}
}

func reportStatusFromProto(value moderationpb.ReportStatus) string {
	switch value {
	case moderationpb.ReportStatus_REPORT_STATUS_PENDING:
		return models.StatusPending
	case moderationpb.ReportStatus_REPORT_STATUS_REVIEWING:
		return models.StatusReviewing
	case moderationpb.ReportStatus_REPORT_STATUS_ACTIONED:
		return models.StatusActioned
	case moderationpb.ReportStatus_REPORT_STATUS_DISMISSED:
		return models.StatusDismissed
	default:
		return ""
	}
}

func reportStatusToProto(value string) moderationpb.ReportStatus {
	switch value {
	case models.StatusPending:
		return moderationpb.ReportStatus_REPORT_STATUS_PENDING
	case models.StatusReviewing:
		return moderationpb.ReportStatus_REPORT_STATUS_REVIEWING
	case models.StatusActioned:
		return moderationpb.ReportStatus_REPORT_STATUS_ACTIONED
	case models.StatusDismissed:
		return moderationpb.ReportStatus_REPORT_STATUS_DISMISSED
	default:
		return moderationpb.ReportStatus_REPORT_STATUS_UNSPECIFIED
	}
}

func toProtoReport(report *models.Report) *moderationpb.Report {
	if report == nil {
		return nil
	}

	pb := &moderationpb.Report{
		Id:         report.ID.String(),
		ReporterId: report.ReporterID.String(),
		TargetType: targetTypeToProto(report.TargetType),
		TargetId:   report.TargetID.String(),
		Reason:     reasonToProto(report.Reason),
		Details:    report.Details,
		Status:     reportStatusToProto(report.Status),
		CreatedAt:  report.CreatedAt.Format(time.RFC3339),
		ReviewNote: report.ReviewNote,
	}
	if report.ReviewedAt != nil {
		pb.ReviewedAt = report.ReviewedAt.Format(time.RFC3339)
	}
	if report.ReviewerID != nil {
		pb.ReviewerId = report.ReviewerID.String()
	}
	return pb
}

func (h *ModerationHandler) BlockUser(ctx context.Context, req *moderationpb.BlockUserRequest) (*moderationpb.ModerationActionResponse, error) {
	if err := h.moderationService.BlockUser(ctx, req.GetBlockerId(), req.GetBlockedId()); err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ModerationActionResponse{Message: "user blocked"}, nil
}

func (h *ModerationHandler) UnblockUser(ctx context.Context, req *moderationpb.BlockUserRequest) (*moderationpb.ModerationActionResponse, error) {
	if err := h.moderationService.UnblockUser(ctx, req.GetBlockerId(), req.GetBlockedId()); err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ModerationActionResponse{Message: "user unblocked"}, nil
}

func (h *ModerationHandler) MuteUser(ctx context.Context, req *moderationpb.MuteUserRequest) (*moderationpb.ModerationActionResponse, error) {
	if err := h.moderationService.MuteUser(ctx, req.GetMuterId(), req.GetMutedId()); err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ModerationActionResponse{Message: "user muted"}, nil
}

func (h *ModerationHandler) UnmuteUser(ctx context.Context, req *moderationpb.MuteUserRequest) (*moderationpb.ModerationActionResponse, error) {
	if err := h.moderationService.UnmuteUser(ctx, req.GetMuterId(), req.GetMutedId()); err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ModerationActionResponse{Message: "user unmuted"}, nil
}

func (h *ModerationHandler) GetModerationStatus(ctx context.Context, req *moderationpb.GetModerationStatusRequest) (*moderationpb.GetModerationStatusResponse, error) {
	visibility, err := h.moderationService.GetStatus(ctx, req.GetViewerId(), req.GetTargetId())
	if err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.GetModerationStatusResponse{
		IsBlocked:        visibility.IsBlocked,
		HasBlockedViewer: visibility.HasBlockedViewer,
		IsMuted:          visibility.IsMuted,
	}, nil
}

func (h *ModerationHandler) ListBlockedUsers(ctx context.Context, req *moderationpb.ListRelationshipsRequest) (*moderationpb.ListRelationshipsResponse, error) {
	ids, err := h.moderationService.ListBlocked(ctx, req.GetUserId())
	if err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ListRelationshipsResponse{UserIds: ids}, nil
}

func (h *ModerationHandler) ListMutedUsers(ctx context.Context, req *moderationpb.ListRelationshipsRequest) (*moderationpb.ListRelationshipsResponse, error) {
	ids, err := h.moderationService.ListMuted(ctx, req.GetUserId())
	if err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ListRelationshipsResponse{UserIds: ids}, nil
}

func (h *ModerationHandler) ReportContent(ctx context.Context, req *moderationpb.ReportContentRequest) (*moderationpb.ReportContentResponse, error) {
	report, duplicate, err := h.moderationService.ReportContent(
		ctx,
		req.GetReporterId(),
		targetTypeFromProto(req.GetTargetType()),
		req.GetTargetId(),
		reasonFromProto(req.GetReason()),
		req.GetDetails(),
	)
	if err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ReportContentResponse{
		Report:    toProtoReport(report),
		Duplicate: duplicate,
	}, nil
}

func (h *ModerationHandler) ListReports(ctx context.Context, req *moderationpb.ListReportsRequest) (*moderationpb.ListReportsResponse, error) {
	page, err := h.moderationService.ListReports(ctx, reportStatusFromProto(req.GetStatus()), req.GetLimit(), req.GetCursor())
	if err != nil {
		return nil, moderationStatus(err)
	}

	reports := make([]*moderationpb.Report, 0, len(page.Reports))
	for i := range page.Reports {
		reports = append(reports, toProtoReport(&page.Reports[i]))
	}

	return &moderationpb.ListReportsResponse{
		Reports:    reports,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (h *ModerationHandler) ResolveReport(ctx context.Context, req *moderationpb.ResolveReportRequest) (*moderationpb.ResolveReportResponse, error) {
	report, err := h.moderationService.ResolveReport(
		ctx,
		req.GetReportId(),
		req.GetReviewerId(),
		reportStatusFromProto(req.GetStatus()),
		req.GetNote(),
	)
	if err != nil {
		return nil, moderationStatus(err)
	}
	return &moderationpb.ResolveReportResponse{Report: toProtoReport(report)}, nil
}
