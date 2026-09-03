package grpcserver

import (
	"context"
	"strings"

	bundlesv1 "live-platform/gen/proto/live/bundles/v1"
	commonv1 "live-platform/gen/proto/live/common/v1"
	couponsv1 "live-platform/gen/proto/live/coupons/v1"
	courseordersv1 "live-platform/gen/proto/live/courseorders/v1"
	feesv1 "live-platform/gen/proto/live/fees/v1"
	subscriptionsv1 "live-platform/gen/proto/live/subscriptions/v1"
	"live-platform/internal/coupons"
	"live-platform/internal/coursebundles"
	"live-platform/internal/courseorders"
	"live-platform/internal/database/db"
	"live-platform/internal/fees"
	"live-platform/internal/subscriptions"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapPaymentErr(err error) error {
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "not found"):
		return status.Error(codes.NotFound, err.Error())
	case strings.Contains(err.Error(), "signature") || strings.Contains(err.Error(), "mismatch"):
		return status.Error(codes.FailedPrecondition, err.Error())
	case strings.Contains(err.Error(), "free or unpriced") || strings.Contains(err.Error(), "already"):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return toStatus(err)
	}
}

// ─────────────────────────────────────────────────────────── course orders

type CourseOrderServer struct {
	courseordersv1.UnimplementedCourseOrderServiceServer
	svc   *courseorders.Service
	keyID string
}

func NewCourseOrderServer(svc *courseorders.Service, keyID string) *CourseOrderServer {
	return &CourseOrderServer{svc: svc, keyID: keyID}
}

func (s *CourseOrderServer) BuyCourse(ctx context.Context, req *courseordersv1.BuyCourseRequest) (*courseordersv1.BuyCourseResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesStudentUp...); err != nil {
		return nil, err
	}
	courseID, err := parseUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	if req.GetAmountMinor() < 0 {
		return nil, invalidArg("amount_minor must be >= 0")
	}
	res, err := s.svc.Buy(ctx, c.TenantID, c.UserID, courseID, s.keyID, courseorders.BuyRequest{
		CouponCode: req.GetCouponCode(), AmountMinor: req.GetAmountMinor(),
	})
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &courseordersv1.BuyCourseResponse{
		OrderId: res.OrderID, Amount: &commonv1.Money{Minor: res.Amount, Currency: res.Currency},
		PaymentRecordId: res.PaymentID, RazorpayKeyId: res.KeyID,
	}, nil
}

func (s *CourseOrderServer) VerifyPayment(ctx context.Context, req *courseordersv1.VerifyPaymentRequest) (*courseordersv1.VerifyPaymentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetRazorpayOrderId() == "" || req.GetRazorpayPaymentId() == "" || req.GetRazorpaySignature() == "" {
		return nil, invalidArg("razorpay_order_id, razorpay_payment_id and razorpay_signature are required")
	}
	res, err := s.svc.Verify(ctx, courseorders.VerifyRequest{
		RazorpayOrderID: req.GetRazorpayOrderId(), RazorpayPaymentID: req.GetRazorpayPaymentId(),
		RazorpaySignature: req.GetRazorpaySignature(),
	}, c.UserID)
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &courseordersv1.VerifyPaymentResponse{Status: res.Status, OrderId: res.OrderID}, nil
}

// ─────────────────────────────────────────────────────────────── bundles

type BundleServer struct {
	bundlesv1.UnimplementedBundleServiceServer
	svc   *coursebundles.Service
	keyID string
}

func NewBundleServer(svc *coursebundles.Service, keyID string) *BundleServer {
	return &BundleServer{svc: svc, keyID: keyID}
}

func bundleMsg(b coursebundles.BundleView) *bundlesv1.Bundle {
	return &bundlesv1.Bundle{
		Id: b.ID, Title: b.Title, Description: b.Description, CoverUrl: b.CoverURL,
		Price: &commonv1.Money{Minor: b.PriceMinor, Currency: "INR"}, IsActive: b.IsActive,
		CourseIds: b.CourseIDs,
	}
}

func (s *BundleServer) ListBundles(ctx context.Context, _ *bundlesv1.ListBundlesRequest) (*bundlesv1.ListBundlesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.List(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &bundlesv1.ListBundlesResponse{}
	for _, b := range rows {
		out.Bundles = append(out.Bundles, bundleMsg(b))
	}
	return out, nil
}

func (s *BundleServer) BuyBundle(ctx context.Context, req *bundlesv1.BuyBundleRequest) (*bundlesv1.BuyBundleResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesStudentUp...); err != nil {
		return nil, err
	}
	bundleID, err := parseUUID(req.GetBundleId(), "bundle_id")
	if err != nil {
		return nil, err
	}
	res, err := s.svc.Buy(ctx, c.TenantID, c.UserID, bundleID, s.keyID)
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &bundlesv1.BuyBundleResponse{
		OrderId: res.OrderID, Amount: &commonv1.Money{Minor: res.Amount, Currency: res.Currency},
		PaymentRecordId: res.PaymentID, RazorpayKeyId: res.KeyID,
	}, nil
}

func (s *BundleServer) VerifyBundlePayment(ctx context.Context, req *bundlesv1.VerifyBundlePaymentRequest) (*bundlesv1.VerifyBundlePaymentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svc.Verify(ctx, coursebundles.VerifyRequest{
		RazorpayOrderID: req.GetRazorpayOrderId(), RazorpayPaymentID: req.GetRazorpayPaymentId(),
		RazorpaySignature: req.GetRazorpaySignature(),
	}, c.UserID); err != nil {
		return nil, mapPaymentErr(err)
	}
	return &bundlesv1.VerifyBundlePaymentResponse{Status: "paid"}, nil
}

func (s *BundleServer) AdminListBundles(ctx context.Context, _ *bundlesv1.AdminListBundlesRequest) (*bundlesv1.AdminListBundlesResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	rows, err := s.svc.AdminList(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &bundlesv1.AdminListBundlesResponse{}
	for _, b := range rows {
		out.Bundles = append(out.Bundles, bundleMsg(b))
	}
	return out, nil
}

func (s *BundleServer) AdminCreateBundle(ctx context.Context, req *bundlesv1.AdminCreateBundleRequest) (*bundlesv1.AdminCreateBundleResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	in := coursebundles.AdminBundleInput{
		Title: req.GetTitle(), Description: req.GetDescription(), CoverURL: req.GetCoverUrl(),
		PriceMinor: req.GetPriceMinor(), DisplayOrder: req.GetDisplayOrder(), CourseIDs: req.GetCourseIds(),
	}
	if req.IsActive != nil {
		v := req.GetIsActive()
		in.IsActive = &v
	}
	b, err := s.svc.AdminCreate(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &bundlesv1.AdminCreateBundleResponse{Bundle: bundleMsg(*b)}, nil
}

func (s *BundleServer) AdminSetBundleActive(ctx context.Context, req *bundlesv1.AdminSetBundleActiveRequest) (*bundlesv1.AdminSetBundleActiveResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.AdminSetActive(ctx, c.TenantID, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &bundlesv1.AdminSetBundleActiveResponse{}, nil
}

func (s *BundleServer) AdminDeleteBundle(ctx context.Context, req *bundlesv1.AdminDeleteBundleRequest) (*bundlesv1.AdminDeleteBundleResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.AdminDelete(ctx, c.TenantID, id); err != nil {
		return nil, toStatus(err)
	}
	return &bundlesv1.AdminDeleteBundleResponse{}, nil
}

// ────────────────────────────────────────────────────────── subscriptions

type SubscriptionServer struct {
	subscriptionsv1.UnimplementedSubscriptionServiceServer
	svc   *subscriptions.Service
	keyID string
}

func NewSubscriptionServer(svc *subscriptions.Service, keyID string) *SubscriptionServer {
	return &SubscriptionServer{svc: svc, keyID: keyID}
}

func planMsg(p subscriptions.PlanView) *subscriptionsv1.Plan {
	return &subscriptionsv1.Plan{
		Id: p.ID, Name: p.Name, Slug: p.Slug, Description: p.Description,
		Price:        &commonv1.Money{Minor: p.PriceMinor, Currency: "INR"},
		DurationDays: p.DurationDays, TrialDays: p.TrialDays, Features: p.Features,
	}
}

func subMsg(v *subscriptions.SubView) *subscriptionsv1.Subscription {
	if v == nil {
		return nil
	}
	return &subscriptionsv1.Subscription{
		Id: v.ID, PlanId: v.PlanID, PlanName: v.PlanName, Status: v.Status, ExpiresAt: tsFromTime(v.ExpiresAt),
	}
}

func (s *SubscriptionServer) ListActivePlans(ctx context.Context, _ *subscriptionsv1.ListActivePlansRequest) (*subscriptionsv1.ListActivePlansResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListActivePlans(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &subscriptionsv1.ListActivePlansResponse{}
	for _, p := range rows {
		out.Plans = append(out.Plans, planMsg(p))
	}
	return out, nil
}

func (s *SubscriptionServer) CreatePlan(ctx context.Context, req *subscriptionsv1.CreatePlanRequest) (*subscriptionsv1.CreatePlanResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	in := subscriptions.UpsertPlanRequest{
		Name: req.GetName(), Slug: req.GetSlug(), Description: req.GetDescription(),
		PriceMinor: req.GetPriceMinor(), DurationDays: req.GetDurationDays(), TrialDays: req.GetTrialDays(),
		Features: req.GetFeatures(), DisplayOrder: req.GetDisplayOrder(),
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	p, err := s.svc.CreatePlan(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &subscriptionsv1.CreatePlanResponse{Plan: planMsg(p)}, nil
}

func (s *SubscriptionServer) SetPlanActive(ctx context.Context, req *subscriptionsv1.SetPlanActiveRequest) (*subscriptionsv1.SetPlanActiveResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetPlanActive(ctx, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &subscriptionsv1.SetPlanActiveResponse{}, nil
}

func (s *SubscriptionServer) StartCheckout(ctx context.Context, req *subscriptionsv1.StartCheckoutRequest) (*subscriptionsv1.StartCheckoutResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	planID, err := parseUUID(req.GetPlanId(), "plan_id")
	if err != nil {
		return nil, err
	}
	res, err := s.svc.StartCheckout(ctx, c.TenantID, c.UserID, subscriptions.CheckoutRequest{PlanID: planID}, s.keyID)
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &subscriptionsv1.StartCheckoutResponse{
		OrderId: res.OrderID, RazorpayOrderId: res.RazorpayOrder,
		Amount: &commonv1.Money{Minor: res.Amount, Currency: res.Currency}, PlanName: res.PlanName,
		RazorpayKeyId: res.PublicKey,
	}, nil
}

func (s *SubscriptionServer) VerifyCheckout(ctx context.Context, req *subscriptionsv1.VerifyCheckoutRequest) (*subscriptionsv1.VerifyCheckoutResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.svc.VerifyCheckout(ctx, c.UserID, subscriptions.VerifyRequest{
		RazorpayOrderID: req.GetRazorpayOrderId(), RazorpayPaymentID: req.GetRazorpayPaymentId(),
		RazorpaySignature: req.GetRazorpaySignature(),
	})
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &subscriptionsv1.VerifyCheckoutResponse{Subscription: subMsg(v)}, nil
}

func (s *SubscriptionServer) GetActiveSubscription(ctx context.Context, _ *subscriptionsv1.GetActiveSubscriptionRequest) (*subscriptionsv1.GetActiveSubscriptionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.svc.GetActive(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &subscriptionsv1.GetActiveSubscriptionResponse{Subscription: subMsg(v)}, nil
}

func (s *SubscriptionServer) ListMySubscriptions(ctx context.Context, _ *subscriptionsv1.ListMySubscriptionsRequest) (*subscriptionsv1.ListMySubscriptionsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &subscriptionsv1.ListMySubscriptionsResponse{}
	for _, r := range rows {
		out.Subscriptions = append(out.Subscriptions, &subscriptionsv1.Subscription{
			Id: r.ID, PlanId: r.PlanID, PlanName: r.PlanName, Status: r.Status,
		})
	}
	return out, nil
}

func (s *SubscriptionServer) CancelSubscription(ctx context.Context, req *subscriptionsv1.CancelSubscriptionRequest) (*subscriptionsv1.CancelSubscriptionResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetSubscriptionId(), "subscription_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Cancel(ctx, c.TenantID, c.UserID, id); err != nil {
		return nil, toStatus(err)
	}
	return &subscriptionsv1.CancelSubscriptionResponse{}, nil
}

// ─────────────────────────────────────────────────────────────── fees

type FeeServer struct {
	feesv1.UnimplementedFeeServiceServer
	svc   *fees.Service
	keyID string
}

func NewFeeServer(svc *fees.Service, keyID string) *FeeServer {
	return &FeeServer{svc: svc, keyID: keyID}
}

func dateStr(d pgDate) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func installmentMsg(id, accID string, seq int32, amount int64, due pgDate, statusStr string, paidAt pgTs, courseTitle string) *feesv1.Installment {
	return &feesv1.Installment{
		Id: id, FeeAccountId: accID, Seq: seq, Amount: &commonv1.Money{Minor: amount, Currency: "INR"},
		DueOn: dateStr(due), Status: statusStr, PaidAt: tsFromPgtz(paidAt), CourseTitle: courseTitle,
	}
}

func (s *FeeServer) ListFeePlans(ctx context.Context, req *feesv1.ListFeePlansRequest) (*feesv1.ListFeePlansResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	out := &feesv1.ListFeePlansResponse{}
	if req.GetCourseId() != "" {
		courseID, perr := parseUUID(req.GetCourseId(), "course_id")
		if perr != nil {
			return nil, perr
		}
		rows, err := s.svc.ListStructuresByCourse(ctx, c.TenantID, courseID)
		if err != nil {
			return nil, toStatus(err)
		}
		for _, p := range rows {
			out.Plans = append(out.Plans, &feesv1.FeePlan{
				Id: utils.UUIDFromPg(p.ID), CourseId: req.GetCourseId(), Name: p.Name,
				Total: &commonv1.Money{Minor: p.TotalMinor, Currency: "INR"}, InstallmentsCount: p.InstallmentsCount,
				GapDays: p.GapDays, IsActive: p.IsActive,
			})
		}
		return out, nil
	}
	rows, err := s.svc.ListStructuresForTenant(ctx, c.TenantID)
	if err != nil {
		return nil, toStatus(err)
	}
	for _, p := range rows {
		out.Plans = append(out.Plans, &feesv1.FeePlan{
			Id: utils.UUIDFromPg(p.ID), CourseId: utils.UUIDFromPg(p.CourseID), CourseTitle: utils.TextFromPg(p.CourseTitle),
			Name: p.Name, Total: &commonv1.Money{Minor: p.TotalMinor, Currency: "INR"},
			InstallmentsCount: p.InstallmentsCount, GapDays: p.GapDays, IsActive: p.IsActive,
		})
	}
	return out, nil
}

func (s *FeeServer) CreateFeePlan(ctx context.Context, req *feesv1.CreateFeePlanRequest) (*feesv1.CreateFeePlanResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	batchID, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	in := fees.CreateFeeStructureRequest{
		CourseID: courseID, BatchID: batchID, Name: req.GetName(), TotalMinor: req.GetTotalMinor(),
		InstallmentsCount: req.GetInstallmentsCount(), InstallmentGapDays: req.GetInstallmentGapDays(),
		LateFeeMinor: req.GetLateFeeMinor(),
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	p, err := s.svc.CreateStructure(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &feesv1.CreateFeePlanResponse{Plan: &feesv1.FeePlan{
		Id: utils.UUIDFromPg(p.ID), CourseId: utils.UUIDFromPg(p.CourseID), Name: p.Name,
		Total: &commonv1.Money{Minor: p.TotalMinor, Currency: "INR"}, InstallmentsCount: p.InstallmentsCount,
		GapDays: p.GapDays, IsActive: p.IsActive,
	}}, nil
}

func (s *FeeServer) AssignFeePlan(ctx context.Context, req *feesv1.AssignFeePlanRequest) (*feesv1.AssignFeePlanResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	userID, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	feePlanID, err := optUUID(req.GetFeePlanId(), "fee_plan_id")
	if err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	batchID, err := optUUID(req.GetBatchId(), "batch_id")
	if err != nil {
		return nil, err
	}
	in := fees.AssignFeeRequest{
		UserID: userID, FeePlanID: feePlanID, CourseID: courseID, BatchID: batchID,
		TotalMinor: req.GetTotalMinor(), Installments: req.GetInstallments(), GapDays: req.GetGapDays(),
	}
	if req.GetFirstDueDate() != nil {
		t := req.GetFirstDueDate().AsTime()
		in.FirstDueDate = &t
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	acc, insts, err := s.svc.Assign(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &feesv1.AssignFeePlanResponse{FeeAccountId: utils.UUIDFromPg(acc.ID)}
	for _, i := range insts {
		out.Installments = append(out.Installments, installmentMsg(
			utils.UUIDFromPg(i.ID), utils.UUIDFromPg(i.FeeAccountID), i.Seq, i.AmountMinor, i.DueOn, string(i.Status), pgTs{}, "",
		))
	}
	return out, nil
}

func (s *FeeServer) ListMyInstallments(ctx context.Context, _ *feesv1.ListMyInstallmentsRequest) (*feesv1.ListMyInstallmentsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListMine(ctx, c.TenantID, c.UserID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &feesv1.ListMyInstallmentsResponse{}
	for _, i := range rows {
		out.Installments = append(out.Installments, installmentMsg(
			utils.UUIDFromPg(i.ID), utils.UUIDFromPg(i.FeeAccountID), i.Seq, i.AmountMinor, i.DueOn,
			string(i.Status), i.PaidAt, utils.TextFromPg(i.CourseTitle),
		))
	}
	return out, nil
}

func (s *FeeServer) GetAccountInstallments(ctx context.Context, req *feesv1.GetAccountInstallmentsRequest) (*feesv1.GetAccountInstallmentsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesInstructorUp...); err != nil {
		return nil, err
	}
	accID, err := parseUUID(req.GetFeeAccountId(), "fee_account_id")
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.GetInstallments(ctx, accID)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &feesv1.GetAccountInstallmentsResponse{}
	for _, i := range rows {
		out.Installments = append(out.Installments, installmentMsg(
			utils.UUIDFromPg(i.ID), req.GetFeeAccountId(), i.Seq, i.AmountMinor, i.DueOn, string(i.Status), i.PaidAt, "",
		))
	}
	return out, nil
}

func (s *FeeServer) StartInstallmentCheckout(ctx context.Context, req *feesv1.StartInstallmentCheckoutRequest) (*feesv1.StartInstallmentCheckoutResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	instID, err := parseUUID(req.GetInstallmentId(), "installment_id")
	if err != nil {
		return nil, err
	}
	res, err := s.svc.StartInstallmentCheckout(ctx, c.TenantID, c.UserID, fees.PayInstallmentRequest{InstallmentID: instID}, s.keyID)
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &feesv1.StartInstallmentCheckoutResponse{
		InstallmentId: res.InstallmentID, OrderId: res.OrderID, RazorpayOrderId: res.RazorpayOrder,
		Amount: &commonv1.Money{Minor: res.Amount, Currency: res.Currency}, RazorpayKeyId: res.PublicKey,
	}, nil
}

func (s *FeeServer) VerifyInstallmentPayment(ctx context.Context, req *feesv1.VerifyInstallmentPaymentRequest) (*feesv1.VerifyInstallmentPaymentResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	instID, err := parseUUID(req.GetInstallmentId(), "installment_id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.VerifyInstallmentPayment(ctx, c.UserID, fees.VerifyInstallmentRequest{
		InstallmentID: instID, RazorpayOrderID: req.GetRazorpayOrderId(),
		RazorpayPaymentID: req.GetRazorpayPaymentId(), RazorpaySignature: req.GetRazorpaySignature(),
	}); err != nil {
		return nil, mapPaymentErr(err)
	}
	return &feesv1.VerifyInstallmentPaymentResponse{}, nil
}

// ─────────────────────────────────────────────────────────────── coupons

type CouponServer struct {
	couponsv1.UnimplementedCouponServiceServer
	svc *coupons.Service
}

func NewCouponServer(svc *coupons.Service) *CouponServer { return &CouponServer{svc: svc} }

func couponMsg(id, code string, typ db.CouponType, pbps pgInt4, val, maxD pgInt8, minOrder int64, appliesTo db.CouponScope, usageLimit pgInt4, perUser, usedCount int32, active bool) *couponsv1.Coupon {
	m := &couponsv1.Coupon{
		Id: id, Code: code, Type: string(typ), MinOrderMinor: minOrder, AppliesTo: string(appliesTo),
		PerUserLimit: perUser, UsedCount: usedCount, IsActive: active,
	}
	if pbps.Valid {
		m.PercentBps = pbps.Int32
	}
	if val.Valid {
		m.ValueMinor = val.Int64
	}
	if maxD.Valid {
		m.MaxDiscountMinor = maxD.Int64
	}
	if usageLimit.Valid {
		m.UsageLimit = usageLimit.Int32
	}
	return m
}

func (s *CouponServer) ListCoupons(ctx context.Context, req *couponsv1.ListCouponsRequest) (*couponsv1.ListCouponsResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	limit, offset := pageArgs(req.GetPage())
	rows, err := s.svc.List(ctx, c.TenantID, limit, offset)
	if err != nil {
		return nil, toStatus(err)
	}
	out := &couponsv1.ListCouponsResponse{}
	for _, r := range rows {
		out.Coupons = append(out.Coupons, couponMsg(utils.UUIDFromPg(r.ID), r.Code, r.Type, r.PercentBps, r.ValueMinor,
			r.MaxDiscountMinor, r.MinOrderMinor, r.AppliesTo, r.UsageLimit, r.PerUserLimit, r.UsedCount, r.IsActive))
	}
	return out, nil
}

func (s *CouponServer) CreateCoupon(ctx context.Context, req *couponsv1.CreateCouponRequest) (*couponsv1.CreateCouponResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	in := coupons.CreateInput{
		Code: req.GetCode(), DiscountType: req.GetDiscountType(), DiscountValue: int(req.GetDiscountValue()),
		Scope: req.GetScope(), MinAmount: int(req.GetMinAmount()),
	}
	if req.GetMaxDiscount() > 0 {
		v := int(req.GetMaxDiscount())
		in.MaxDiscount = &v
	}
	if req.GetUsageLimit() > 0 {
		v := int(req.GetUsageLimit())
		in.UsageLimit = &v
	}
	if req.GetPerUserLimit() > 0 {
		v := int(req.GetPerUserLimit())
		in.PerUserLimit = &v
	}
	for _, cid := range req.GetCourseIds() {
		id, perr := uuid.Parse(cid)
		if perr != nil {
			return nil, invalidArg("course_ids must be uuids")
		}
		in.CourseIDs = append(in.CourseIDs, id)
	}
	if err := validate(&in); err != nil {
		return nil, err
	}
	r, err := s.svc.Create(ctx, c.TenantID, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &couponsv1.CreateCouponResponse{Coupon: couponMsg(utils.UUIDFromPg(r.ID), r.Code, r.Type, r.PercentBps, r.ValueMinor,
		r.MaxDiscountMinor, r.MinOrderMinor, r.AppliesTo, r.UsageLimit, r.PerUserLimit, r.UsedCount, r.IsActive)}, nil
}

func (s *CouponServer) SetCouponActive(ctx context.Context, req *couponsv1.SetCouponActiveRequest) (*couponsv1.SetCouponActiveResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetActive(ctx, c.TenantID, id, req.GetActive()); err != nil {
		return nil, toStatus(err)
	}
	return &couponsv1.SetCouponActiveResponse{}, nil
}

func (s *CouponServer) DeleteCoupon(ctx context.Context, req *couponsv1.DeleteCouponRequest) (*couponsv1.DeleteCouponResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.require(rolesAdminOnly...); err != nil {
		return nil, err
	}
	id, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, c.TenantID, id); err != nil {
		return nil, toStatus(err)
	}
	return &couponsv1.DeleteCouponResponse{}, nil
}

func (s *CouponServer) ApplyCoupon(ctx context.Context, req *couponsv1.ApplyCouponRequest) (*couponsv1.ApplyCouponResponse, error) {
	c, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	courseID, err := optUUID(req.GetCourseId(), "course_id")
	if err != nil {
		return nil, err
	}
	res, err := s.svc.Apply(ctx, c.TenantID, c.UserID, req.GetCode(), int(req.GetAmountMinor()), courseID, req.GetIsSubscription())
	if err != nil {
		return nil, mapPaymentErr(err)
	}
	return &couponsv1.ApplyCouponResponse{
		CouponId: res.CouponID.String(), Code: res.Code,
		AmountOffMinor: int64(res.AmountOff), FinalAmountMinor: int64(res.Final),
	}, nil
}
