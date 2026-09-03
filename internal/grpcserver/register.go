package grpcserver

import (
	adminv1 "live-platform/gen/proto/live/admin/v1"
	analyticsv1 "live-platform/gen/proto/live/analytics/v1"
	assignmentsv1 "live-platform/gen/proto/live/assignments/v1"
	attendancev1 "live-platform/gen/proto/live/attendance/v1"
	auditv1 "live-platform/gen/proto/live/audit/v1"
	authv1 "live-platform/gen/proto/live/auth/v1"
	bannersv1 "live-platform/gen/proto/live/banners/v1"
	batchesv1 "live-platform/gen/proto/live/batches/v1"
	billingv1 "live-platform/gen/proto/live/billing/v1"
	bookmarksv1 "live-platform/gen/proto/live/bookmarks/v1"
	bundlesv1 "live-platform/gen/proto/live/bundles/v1"
	chaptersv1 "live-platform/gen/proto/live/chapters/v1"
	cmsv1 "live-platform/gen/proto/live/cms/v1"
	couponsv1 "live-platform/gen/proto/live/coupons/v1"
	courseordersv1 "live-platform/gen/proto/live/courseorders/v1"
	coursesv1 "live-platform/gen/proto/live/courses/v1"
	devicesv1 "live-platform/gen/proto/live/devices/v1"
	doubtsv1 "live-platform/gen/proto/live/doubts/v1"
	engagementv1 "live-platform/gen/proto/live/engagement/v1"
	enrollmentsv1 "live-platform/gen/proto/live/enrollments/v1"
	examsv1 "live-platform/gen/proto/live/exams/v1"
	feesv1 "live-platform/gen/proto/live/fees/v1"
	leadsv1 "live-platform/gen/proto/live/leads/v1"
	lecturesv1 "live-platform/gen/proto/live/lectures/v1"
	notificationsv1 "live-platform/gen/proto/live/notifications/v1"
	platformadminv1 "live-platform/gen/proto/live/platformadmin/v1"
	referralsv1 "live-platform/gen/proto/live/referrals/v1"
	schedulev1 "live-platform/gen/proto/live/schedule/v1"
	searchv1 "live-platform/gen/proto/live/search/v1"
	subjectsv1 "live-platform/gen/proto/live/subjects/v1"
	subscriptionsv1 "live-platform/gen/proto/live/subscriptions/v1"
	tenantsv1 "live-platform/gen/proto/live/tenants/v1"
	testsv1 "live-platform/gen/proto/live/tests/v1"
	topicsv1 "live-platform/gen/proto/live/topics/v1"
	usersv1 "live-platform/gen/proto/live/users/v1"
	"live-platform/internal/admin"
	"live-platform/internal/analytics"
	"live-platform/internal/assignments"
	"live-platform/internal/attendance"
	"live-platform/internal/audit"
	"live-platform/internal/auth"
	"live-platform/internal/banners"
	"live-platform/internal/batches"
	"live-platform/internal/billing"
	"live-platform/internal/bookmarks"
	"live-platform/internal/chapters"
	"live-platform/internal/cms"
	"live-platform/internal/config"
	"live-platform/internal/coupons"
	"live-platform/internal/coursebundles"
	"live-platform/internal/courseorders"
	"live-platform/internal/courses"
	"live-platform/internal/devices"
	"live-platform/internal/doubts"
	"live-platform/internal/engagement"
	"live-platform/internal/enrollments"
	"live-platform/internal/exams"
	"live-platform/internal/fees"
	"live-platform/internal/leads"
	"live-platform/internal/lectures"
	"live-platform/internal/notifications"
	"live-platform/internal/platformadmin"
	"live-platform/internal/referrals"
	"live-platform/internal/schedule"
	"live-platform/internal/search"
	"live-platform/internal/subjects"
	"live-platform/internal/subscriptions"
	"live-platform/internal/tenants"
	"live-platform/internal/tests"
	"live-platform/internal/topics"
	"live-platform/internal/users"

	"google.golang.org/grpc"
)

// registerAll wires every domain adapter onto s. Each service is constructed
// exactly as cmd/server/main.go builds it (same builder chains), reusing the
// shared clients on d.
func registerAll(s *grpc.Server, cfg *config.Config, d Deps) {
	referralSvc := referrals.NewService(d.Pool)

	// --- Catalog / taxonomy ---
	coursesv1.RegisterCourseServiceServer(s, NewCourseServer(courses.NewService(d.Pool)))
	subjectsv1.RegisterSubjectServiceServer(s, NewSubjectServer(subjects.NewService(d.Pool)))
	chaptersv1.RegisterChapterServiceServer(s, NewChapterServer(chapters.NewService(d.Pool)))
	topicsv1.RegisterTopicServiceServer(s, NewTopicServer(topics.NewService(d.Pool)))
	examsv1.RegisterExamCategoryServiceServer(s, NewExamCategoryServer(exams.NewService(d.Pool)))
	batchesv1.RegisterBatchServiceServer(s, NewBatchServer(batches.NewService(d.Pool)))

	// --- Learning ---
	enrollmentsv1.RegisterEnrollmentServiceServer(s, NewEnrollmentServer(enrollments.NewService(d.Pool)))
	bookmarksv1.RegisterBookmarkServiceServer(s, NewBookmarkServer(bookmarks.NewService(d.Pool)))
	lecturesv1.RegisterLectureServiceServer(s, NewLectureServer(lectures.NewService(d.Pool)))
	doubtsv1.RegisterDoubtServiceServer(s, NewDoubtServer(doubts.NewService(d.Pool, d.Claude)))
	attendancev1.RegisterAttendanceServiceServer(s, NewAttendanceServer(attendance.NewService(d.Pool)))
	assignmentsv1.RegisterAssignmentServiceServer(s, NewAssignmentServer(assignments.NewService(d.Pool)))

	// --- Identity / tenant ops ---
	usersv1.RegisterUserServiceServer(s, NewUserServer(users.NewService(d.Pool)))
	devicesv1.RegisterDeviceServiceServer(s, NewDeviceServer(devices.NewService(d.Pool)))
	auditv1.RegisterAuditServiceServer(s, NewAuditServer(audit.NewService(d.Pool)))
	adminv1.RegisterAdminServiceServer(s, NewAdminServer(admin.NewService(d.Pool)))
	referralsv1.RegisterReferralServiceServer(s, NewReferralServer(referralSvc))

	// --- Engagement / comms / growth ---
	searchv1.RegisterSearchServiceServer(s, NewSearchServer(search.NewService(d.Pool)))
	notificationsv1.RegisterNotificationServiceServer(s, NewNotificationServer(notifications.NewService(d.Pool)))
	bannersv1.RegisterBannerServiceServer(s, NewBannerServer(banners.NewService(d.Pool)))
	leadsv1.RegisterLeadServiceServer(s, NewLeadServer(leads.NewService(d.Pool)))
	schedulev1.RegisterScheduleServiceServer(s, NewScheduleServer(schedule.NewService(d.Pool)))

	// --- Commerce ---
	keyID := cfg.Razorpay.KeyID
	courseordersv1.RegisterCourseOrderServiceServer(s, NewCourseOrderServer(
		courseorders.NewService(d.Pool, d.Razorpay).WithProducer(d.Kafka).WithReferrals(referralSvc), keyID))
	bundlesv1.RegisterBundleServiceServer(s, NewBundleServer(
		coursebundles.NewService(d.Pool, d.Razorpay).WithProducer(d.Kafka), keyID))
	subscriptionsv1.RegisterSubscriptionServiceServer(s, NewSubscriptionServer(
		subscriptions.NewService(d.Pool, d.Razorpay), keyID))
	feesv1.RegisterFeeServiceServer(s, NewFeeServer(fees.NewService(d.Pool, d.Razorpay), keyID))
	couponsv1.RegisterCouponServiceServer(s, NewCouponServer(coupons.NewService(d.Pool)))
	billingv1.RegisterBillingServiceServer(s, NewBillingServer(billing.NewService(d.Pool)))

	// --- Analytics / tenant / cms / engagement ---
	analyticsv1.RegisterAnalyticsServiceServer(s, NewAnalyticsServer(analytics.NewService(d.Pool)))
	tenantsv1.RegisterTenantServiceServer(s, NewTenantServer(tenants.NewService(d.Pool)))
	cmsv1.RegisterCmsServiceServer(s, NewCmsServer(cms.NewService(d.Pool)))
	engagementv1.RegisterEngagementServiceServer(s, NewEngagementServer(engagement.NewService(d.Pool)))

	// --- Auth / platform ---
	authSvc := auth.NewService(d.Pool, d.Redis, cfg).WithSMS(d.SMS).WithReferrer(referralSvc).WithGoogle(d.Google)
	authv1.RegisterAuthServiceServer(s, NewAuthServer(authSvc))
	platformadminv1.RegisterPlatformAdminServiceServer(s, NewPlatformAdminServer(
		platformadmin.NewService(d.Pool).WithRazorpay(d.Razorpay), cfg.JWT.AccessSecret))
	testsv1.RegisterTestServiceServer(s, NewTestServer(tests.NewService(d.Pool)))
}
