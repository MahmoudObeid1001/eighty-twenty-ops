package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"
	"eighty-twenty-ops/internal/util"

	"github.com/google/uuid"
)

type PreEnrolmentHandler struct {
	cfg *config.Config
}

func NewPreEnrolmentHandler(cfg *config.Config) *PreEnrolmentHandler {
	return &PreEnrolmentHandler{cfg: cfg}
}

func buildColdLevelOptions(leads []*models.LeadListItem) []int {
	levelsSet := make(map[int]struct{})
	for _, item := range leads {
		if item == nil || item.Lead == nil || !item.AssignedLevel.Valid {
			continue
		}
		level := int(item.AssignedLevel.Int32)
		if level < 1 || level > 10 {
			continue
		}
		levelsSet[level] = struct{}{}
	}
	levels := make([]int, 0, len(levelsSet))
	for level := range levelsSet {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

type waitingListBucket struct {
	Level          int
	ClassDays      string
	ClassTime      string
	Count          int
	MissingToReady int
	HasSchedule    bool
}

type refusalReasonTab struct {
	Key   string
	Label string
}

type renewalPendingMessageDecision struct {
	Key              string
	Label            string
	Text             string
	Outcome          string
	Level            int
	AttendedSessions int
}

func buildWaitingListBuckets(leads []*models.LeadListItem) []waitingListBucket {
	type bucketKey struct {
		level     int
		classDays string
		classTime string
	}

	counts := make(map[bucketKey]int)
	for _, item := range leads {
		if item == nil || item.Lead == nil || item.Lead.Status != "waiting_for_round" {
			continue
		}
		level := 0
		if item.AssignedLevel.Valid {
			level = int(item.AssignedLevel.Int32)
		}
		days := ""
		if item.ClassDays.Valid {
			days = item.ClassDays.String
		}
		timeVal := ""
		if item.ClassTime.Valid {
			timeVal = item.ClassTime.String
		}
		counts[bucketKey{level: level, classDays: days, classTime: timeVal}]++
	}

	buckets := make([]waitingListBucket, 0, len(counts))
	for key, count := range counts {
		missingToReady := 0
		if count < 4 {
			missingToReady = 4 - count
		}
		buckets = append(buckets, waitingListBucket{
			Level:          key.level,
			ClassDays:      key.classDays,
			ClassTime:      key.classTime,
			Count:          count,
			MissingToReady: missingToReady,
			HasSchedule:    key.classDays != "" && key.classTime != "",
		})
	}

	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Level != buckets[j].Level {
			return buckets[i].Level < buckets[j].Level
		}
		if buckets[i].ClassDays != buckets[j].ClassDays {
			return buckets[i].ClassDays < buckets[j].ClassDays
		}
		return buckets[i].ClassTime < buckets[j].ClassTime
	})

	return buckets
}

func filterLeadsByAssignedLevel(leads []*models.LeadListItem, selectedLevel int) []*models.LeadListItem {
	if selectedLevel < 1 || selectedLevel > 10 {
		return leads
	}
	filtered := make([]*models.LeadListItem, 0, len(leads))
	for _, item := range leads {
		if item == nil || !item.AssignedLevel.Valid {
			continue
		}
		if int(item.AssignedLevel.Int32) == selectedLevel {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func isValidAssignedLevel(level int) bool {
	return level >= 1 && level <= 10
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNameFromFullName(fullName string) string {
	fields := strings.Fields(strings.TrimSpace(fullName))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func normalizeWhatsAppPhone(phone string) string {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(phone) {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	digitsOnly := digits.String()
	if strings.HasPrefix(digitsOnly, "00") {
		digitsOnly = strings.TrimPrefix(digitsOnly, "00")
	}
	if strings.HasPrefix(digitsOnly, "0") {
		digitsOnly = "20" + strings.TrimPrefix(digitsOnly, "0")
	}
	return digitsOnly
}

func buildWhatsAppComposeLink(phone, text string) string {
	normalizedPhone := normalizeWhatsAppPhone(phone)
	if normalizedPhone == "" {
		return ""
	}
	base := "https://api.whatsapp.com/send?phone=" + normalizedPhone
	if strings.TrimSpace(text) == "" {
		return base
	}
	return base + "&text=" + url.QueryEscape(text)
}

func buildOfferSentFollowUpMessage(studentFullName string, step int) string {
	studentFirstName := firstNameFromFullName(studentFullName)
	if studentFirstName == "" {
		studentFirstName = "Ahmed"
	}

	switch step {
	case 1:
		return fmt.Sprintf(`%s، 👋

بعتنالك نتيجة البليسمنت تيست والأوفر المناسب ليك،
ولما ماجاش رد حبيت أتأكد إن الرسالة وصلتك 😊

مستواك كويس، وعندك أساس تقدر تبني عليه 💪
ومحتاج بس الخطوة الصح عشان توصل لنتيجة حقيقية.

الأوفر لسه متاح.
ينفع نتكلم فيه؟`, studentFirstName)
	case 2:
		return fmt.Sprintf(`%s، 🎯

هقولك على حاجة:
ناس كتير بتعمل البليسمنت تيست وبعدها بتقف،
مش لأنهم مش عايزين يتعلموا،
لكن لأن بيكون فيه سؤال لسه شاغلهم.

سواء كان الموضوع سعر، وقت، أو حتى هل الكورس مناسب ليك فعلًا،
ابعتلي اللي في بالك وأنا هرد عليك بصراحة تامة، من غير أي إلزام 😊

وصولك لمرحلة التيست معناه إن عندك جدية حقيقية في التطوير 💙`, studentFirstName)
	case 3:
		return fmt.Sprintf(`%s، 😊

دي آخر مرة هتواصل فيها معاك، وبعدها هسيبلك المساحة براحتك.

لكن قبل ما أقفل الملف، حبيت أقدملك فرصة مناسبة ليك:
🎁 أول محاضرة مجانًا، عشان تشوف بنفسك الأسلوب والمدرس قبل أي قرار
🎁 وخصم 15%% على أي باكدج لو حبيت تكمل خلال الأسبوع ده

مفيش أي إلزام في المحاضرة الأولى،
ولو ماعجبتكش، مش هتدفع أي حاجة.

ده عرض بنقدمه تقديرًا إنك خدت خطوة فعلية وعملت التيست 💙

لو مناسب ليك، ابعتلي:
عايز أجرب ✅`, studentFirstName)
	default:
		return ""
	}
}

func buildLeadWhatsAppURL(item *models.LeadListItem) string {
	if item == nil || item.Lead == nil {
		return ""
	}
	return fmt.Sprintf("/pre-enrolment/%s?open_whatsapp=1", item.Lead.ID.String())
}

func assignLeadWhatsAppURLs(items []*models.LeadListItem) {
	for _, item := range items {
		if item == nil {
			continue
		}
		item.WhatsAppURL = buildLeadWhatsAppURL(item)
	}
}

func refusedRenewalReasonTabs() []refusalReasonTab {
	return []refusalReasonTab{
		{Key: models.RefusedRenewalReasonTimePressure, Label: "Busy / Exams"},
		{Key: models.RefusedRenewalReasonFinancial, Label: "Financial"},
		{Key: models.RefusedRenewalReasonNotSatisfied, Label: "Not satisfied"},
		{Key: models.RefusedRenewalReasonOther, Label: "Other"},
	}
}

func refusalReasonLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case models.RefusedRenewalReasonTimePressure:
		return "Busy / Exams"
	case models.RefusedRenewalReasonFinancial:
		return "Financial reasons"
	case models.RefusedRenewalReasonNotSatisfied:
		return "Not satisfied"
	case models.RefusedRenewalReasonOther:
		return "Other"
	default:
		return ""
	}
}

func buildRenewalPendingMessage(fullName string, level int, outcome string, attendedSessions int) (*renewalPendingMessageDecision, bool) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		fullName = "حضرتك"
	}
	if level <= 0 {
		return nil, false
	}

	outcome = strings.TrimSpace(outcome)
	switch {
	case strings.EqualFold(outcome, "promoted"):
		return &renewalPendingMessageDecision{
			Key:              "promoted",
			Label:            "Promoted",
			Outcome:          outcome,
			Level:            level,
			AttendedSessions: attendedSessions,
			Text: fmt.Sprintf(`السلام عليكم استاذ %s، 🎉
حضرتك خصلت معانا ليفل %d بنجاح ما شاء الله ،وبنباركلك على تقدمك للمستوى 🎉
صراحةً حضورك وجديتك كانوا واضحين من أول يوم، والمنتور بيشكر فيك جدا💪
دلوقتي هو أفضل وقت تكمل — ونبني ع الانجاز اللي حققته
تحب نرتب للمستوى الجاي؟ 😊`, fullName, level),
		}, true
	case strings.EqualFold(outcome, "repeated") && attendedSessions >= 3:
		return &renewalPendingMessageDecision{
			Key:              "repeated_partial_attendance",
			Label:            "Repeated - Partial Attendance",
			Outcome:          outcome,
			Level:            level,
			AttendedSessions: attendedSessions,
			Text: fmt.Sprintf(`السلام عليكم استاذ %s، 👋
المستوى %d خلص، وعندي كلام مهم لحضرتك
صحيح الحضور كان متقطع الفترة دي — بس السيشنز اللي حضرتها كان واضح إنك جاد وعندك هدف 💪
وعشان كده مش عايزك تعيد نفس المستوى من غير ما تاخد فرصة حقيقية تكمله صح
عندنا ليك:
🎁 خصم ٤٠٪ على إعادة المستوى %d — عشان تكمل اللي بدأته بالراحة والتركيز اللي يستاهله
الأوفر ده مش بنعلن عنه، وهو بس ليك لأن إحنا شايفين إنك تستاهل الفرصة دي 💙
تحب نحجز مكان لليفل الجديد؟ 😊`, fullName, level, level),
		}, true
	case strings.EqualFold(outcome, "repeated"):
		return &renewalPendingMessageDecision{
			Key:              "repeated_low_attendance",
			Label:            "Repeated - Low Attendance",
			Outcome:          outcome,
			Level:            level,
			AttendedSessions: attendedSessions,
			Text: fmt.Sprintf(`السلام عليكم استاذ %s،
حضرتك كنت مشترك معانا في ليفل %d وحاليا الليفل خلص، وحبيت أتواصل معاك لان بصراحة لاحظنا إن حضورك كان محدود الفترة دي — وده خلانا نفكر ازاي نقدر نساعدك ، بس عايزين نفهم — في حاجة حصلت أو في اي ظرف وقفك؟ دا هيساعدنا نعرف نفيدك أحسن 😊`, fullName, level),
		}, true
	default:
		return nil, false
	}
}

func renewalPendingLabelForKey(key string) string {
	switch strings.TrimSpace(key) {
	case "promoted":
		return "Promoted"
	case "repeated_low_attendance":
		return "Repeated - Low Attendance"
	case "repeated_partial_attendance":
		return "Repeated - Partial Attendance"
	default:
		return ""
	}
}

func applyRenewalPendingMessageToLead(item *models.LeadListItem) {
	if item == nil || item.Lead == nil {
		return
	}
	item.RenewalPendingMessageKey = ""
	item.RenewalPendingMessageLabel = ""
	item.RenewalPendingMessageText = ""
	if item.Lead.Status != "renewal_pending" || !item.Lead.IsReturning || !item.LastOutcome.Valid || !item.LatestCompletedLevel.Valid {
		return
	}

	decision, ok := buildRenewalPendingMessage(item.Lead.FullName, int(item.LatestCompletedLevel.Int32), item.LastOutcome.String, item.LatestAttendedSessions)
	if !ok {
		return
	}
	item.RenewalPendingMessageKey = decision.Key
	item.RenewalPendingMessageLabel = decision.Label
	item.RenewalPendingMessageText = decision.Text
}

func groupRefusedRenewalTemplatesByReason(items []*models.RefusedRenewalMessageTemplate) map[string][]*models.RefusedRenewalMessageTemplate {
	grouped := map[string][]*models.RefusedRenewalMessageTemplate{
		models.RefusedRenewalReasonTimePressure: {},
		models.RefusedRenewalReasonFinancial:    {},
		models.RefusedRenewalReasonNotSatisfied: {},
		models.RefusedRenewalReasonOther:        {},
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		grouped[item.RefusalReason] = append(grouped[item.RefusalReason], item)
	}
	return grouped
}

func countDueRefusedRenewalBannerItems(leads []*models.LeadListItem) int {
	count := 0
	for _, item := range leads {
		if item == nil || item.Lead == nil || !item.RefusedRenewal {
			continue
		}
		if item.RefusedFollowUpDueNow && item.RefusedFollowUpStep > 0 && item.RefusedFollowUpStep < 3 {
			count++
		}
	}
	return count
}

func buildSleepingLeadMessage(studentFullName string, step int) string {
	studentFirstName := firstNameFromFullName(studentFullName)
	if studentFirstName == "" {
		studentFirstName = "صديقنا"
	}

	switch step {
	case 1:
		return fmt.Sprintf(`مرحبا %s 😊
أنا احمد من إيتي توينتي

لاحظت إنك كنت بتسأل عن الكورس وبعتلنا رقمك عشان تعمل البليسمنت تيست..
عايز أعرف، في أي حاجة وقفتك؟ 🤔

أنا هنا لو في أي سؤال، كلمني براحتك 😊`, studentFirstName)
	case 2:
		return fmt.Sprintf(`%s، عارف إيه اللي بيفرق بين الناس اللي بتتكلم إنجليزي كويس.. والناس اللي لسه بتحاول؟ 💡

مش الموهبة، مش الوقت - هو البداية الصح ✅

عندنا دلوقتي طلاب بدأوا زيك بالظبط، وبعد شهرين بيتكلموا في شغلهم وانترفيوهات بثقة 💪

البليسمنت تيست بتاعنا مجاني ومش بياخد أكتر من 15 دقيقة، وبيوريلك بالظبط أنت فين وإيه الخطوة الجاية 🎯

نحجزه امتى بالنسبالك؟`, studentFirstName)
	case 3:
		return fmt.Sprintf(`%s، مش عايز آخد وقتك كتير 😊

بس عارفك مهتم وعايز تطور إنجليزيتك، وده بالنسبالي كفيل إني أبعتلك الأوفر ده:

🎁 أول محاضرة مجانًا + خصم ١٥٪ على أي باكدج لو اشتركت الأسبوع ده

الأوفر ده مش بنعلن عنه، بس بنديه للناس اللي بتفضل معانا ومش قادرة تبدأ لأسباب تانية 💙

كلمني بـ "مهتم" وأنا هرتب معاك كل حاجة في 5 دقايق 🙏

بعد كده هسيبك براحتك، ومتترددش ترجع لو غيرت رأيك في أي وقت 😄`, studentFirstName)
	default:
		return ""
	}
}

var groupBundlePrices = map[int32]int32{
	1: 1250,
	2: 2400,
	3: 3300,
	4: 4000,
}

var privateBundlePrices = map[int32]int32{
	1: 3000,
	2: 5600,
	3: 7800,
}

func normalizePricingTrack(track string) string {
	switch strings.ToLower(strings.TrimSpace(track)) {
	case "private":
		return "private"
	default:
		return "group"
	}
}

func pricingTrackBundlePrices(track string) map[int32]int32 {
	if normalizePricingTrack(track) == "private" {
		return privateBundlePrices
	}
	return groupBundlePrices
}

func inferOfferPricingTrack(offer *models.Offer) string {
	if offer == nil || !offer.BundleLevels.Valid || !offer.BasePrice.Valid {
		return "group"
	}
	if price, ok := privateBundlePrices[offer.BundleLevels.Int32]; ok && offer.BasePrice.Int32 == price {
		return "private"
	}
	return "group"
}

func isRefundReviewLead(lead *models.Lead) bool {
	return lead != nil && lead.OpsQueueReason.Valid && lead.OpsQueueReason.String == "refund_review"
}

func isStatusAtOrAfterOfferSent(status string) bool {
	allowed := map[string]bool{
		"offer_sent":        true,
		"booking_confirmed": true,
		"deposit_paid":      true,
		"paid_full":         true,
		"waiting_for_round": true,
		"ready_to_start":    true,
		"in_classes":        true,
	}
	return allowed[status]
}

func (h *PreEnrolmentHandler) List(w http.ResponseWriter, r *http.Request) {
	// Read filter parameters from query string
	statusFilter := r.URL.Query().Get("status")
	searchFilter := r.URL.Query().Get("search")
	paymentFilter := r.URL.Query().Get("payment")
	hotFilter := r.URL.Query().Get("hot") // Changed from "filter" to "hot"
	returningFilter := r.URL.Query().Get("returning")
	followUpFilter := r.URL.Query().Get("follow_up") // Milestone 2: high_priority follow-up filter
	coldFilter := r.URL.Query().Get("cold")
	coldLevelFilter := r.URL.Query().Get("cold_level")
	repeatFilter := r.URL.Query().Get("repeat")
	opsQueueFilter := r.URL.Query().Get("ops_queue")
	sleepingFilter := r.URL.Query().Get("sleeping")
	snoozedFilter := r.URL.Query().Get("snoozed")
	isSleepingLeads := sleepingFilter == "1" || strings.EqualFold(sleepingFilter, "true")
	isSnoozedLeads := snoozedFilter == "1" || strings.EqualFold(snoozedFilter, "true")
	includeCancelled := r.URL.Query().Get("include_cancelled") == "1" || r.URL.Query().Get("include_cancelled") == "true"
	// When explicitly filtering by status=cancelled, include cancelled even if checkbox off
	if statusFilter == "cancelled" {
		includeCancelled = true
	}
	// Normalize "ALL" filters to empty (no filter)
	if strings.EqualFold(statusFilter, "all") {
		statusFilter = ""
	}
	if strings.EqualFold(paymentFilter, "all") {
		paymentFilter = ""
	}
	if r.URL.Query().Get("dismiss_refused_banner") == "1" {
		var actorID *uuid.UUID
		if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
			if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
				actorID = &parsed
			}
		}
		if err := models.DismissGlobalBannerForDate(models.RefusedRenewalBannerKey, util.CairoStartOfDay(util.CairoNow()), actorID); err != nil {
			log.Printf("ERROR: Failed to dismiss refused renewal banner: %v", err)
		}
		params := r.URL.Query()
		params.Del("dismiss_refused_banner")
		u := "/pre-enrolment"
		if encoded := params.Encode(); encoded != "" {
			u += "?" + encoded
		}
		http.Redirect(w, r, u, http.StatusFound)
		return
	}

	// Check for flash messages in query params (separate from filter status)
	flashMessage, flashMessageType := flashFromQuery(r)
	savedParam := r.URL.Query().Get("saved")
	deletedParam := r.URL.Query().Get("deleted")
	statusFlashParam := r.URL.Query().Get("status_flash")
	sentToClassesParam := r.URL.Query().Get("sentToClasses")

	if flashMessage == "" && sentToClassesParam == "1" {
		flashMessage = "Lead sent to classes board successfully!"
		flashMessageType = "success"
	} else if flashMessage == "" && deletedParam == "1" {
		flashMessage = "Lead cancelled successfully!"
		flashMessageType = "success"
	} else if flashMessage == "" && r.URL.Query().Get("cancelled") == "1" {
		flashMessage = "Lead cancelled successfully!"
		flashMessageType = "success"
	} else if flashMessage == "" && savedParam == "1" {
		flashMessage = "Lead saved successfully!"
		flashMessageType = "success"
	} else if flashMessage == "" && statusFlashParam != "" {
		statusMessages := map[string]string{
			"test_booked": "Placement test booked successfully!",
			"tested":      "Lead marked as tested!",
			"offer_sent":  "Offer sent successfully!",
			"waiting":     "Lead moved to waiting list!",
			"ready":       "Lead marked as ready to start!",
			"cold":        "Lead sent to cold leads.",
			"refused":     "Lead marked as refused renewal and moved to cold leads.",
		}
		if msg, ok := statusMessages[statusFlashParam]; ok {
			flashMessage = msg
			flashMessageType = "success"
		}
	}

	h.cfg.Debugf("List: statusFilter=%q, searchFilter=%q, paymentFilter=%q, hotFilter=%q, followUpFilter=%q, includeCancelled=%v, returningFilter=%q, coldFilter=%q, repeatFilter=%q", statusFilter, searchFilter, paymentFilter, hotFilter, followUpFilter, includeCancelled, returningFilter, coldFilter, repeatFilter)

	// Get filtered leads
	var leads []*models.LeadListItem
	var err error
	if isSnoozedLeads {
		leads, err = models.GetSnoozedLeads(searchFilter)
	} else if isSleepingLeads {
		leads, err = models.GetSleepingLeads(searchFilter)
	} else {
		leads, err = models.GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter, includeCancelled, followUpFilter, returningFilter, coldFilter, repeatFilter, opsQueueFilter)
	}
	if err != nil {
		log.Printf("ERROR: Failed to load leads: %v", err)
		if flashMessage == "" {
			flashMessage = "Couldn't load leads. Please refresh and try again."
			flashMessageType = "error"
		}
		leads = []*models.LeadListItem{}
	}

	h.cfg.Debugf("List: returned %d leads", len(leads))

	var coldLevelOptions []int
	selectedColdLevel := 0
	if !isSleepingLeads && !isSnoozedLeads && (coldFilter == "1" || strings.EqualFold(coldFilter, "true")) {
		coldLevelOptions = buildColdLevelOptions(leads)
		if lvl, err := strconv.Atoi(strings.TrimSpace(coldLevelFilter)); err == nil && isValidAssignedLevel(lvl) {
			selectedColdLevel = lvl
			leads = filterLeadsByAssignedLevel(leads, selectedColdLevel)
		}
	}

	assignLeadWhatsAppURLs(leads)
	for _, item := range leads {
		applyRenewalPendingMessageToLead(item)
	}
	isWaitingListView := strings.EqualFold(statusFilter, "WAITING_FOR_ROUND") || strings.EqualFold(statusFilter, "waiting_for_round")
	waitingListBuckets := []waitingListBucket{}
	if isWaitingListView {
		waitingListBuckets = buildWaitingListBuckets(leads)
	}

	// Count follow-ups due for banner
	// Get total count of hot leads (need to fetch all leads without hot filter)
	var followUpCount int
	if hotFilter == "1" || hotFilter == "hot" {
		// All leads in filtered list are hot leads
		followUpCount = len(leads)
	} else {
		// Get all leads to count hot leads accurately (exclude cancelled)
		allLeads, err := models.GetAllLeads("", "", "", "", false, "", "", "", "", "")
		if err == nil {
			for _, lead := range allLeads {
				if lead.FollowUpDue {
					followUpCount++
				}
			}
		}
	}

	// Count tested leads for "New test results" banner
	testedResultsCount := 0
	if testedLeads, err := models.GetAllLeads("TESTED", "", "", "", false, "", "", "", "", ""); err == nil {
		testedResultsCount = len(testedLeads)
	}

	sleepingReminderDueCount := 0
	if count, err := models.CountDueSleepingLeadReminders(util.CairoNow()); err == nil {
		sleepingReminderDueCount = count
	}

	refusedTemplates, err := models.GetRefusedRenewalMessageTemplates()
	if err != nil {
		log.Printf("ERROR: Failed to load refused renewal templates: %v", err)
		refusedTemplates = []*models.RefusedRenewalMessageTemplate{}
	}
	refusedTemplatesByReason := groupRefusedRenewalTemplatesByReason(refusedTemplates)
	refusedRenewalDueCount := 0
	refusedRenewalBannerDismissed := false
	if coldLeads, coldErr := models.GetAllLeads("", "", "", "", false, "", "", "1", "", ""); coldErr == nil {
		refusedRenewalDueCount = countDueRefusedRenewalBannerItems(coldLeads)
		if dismissed, dismissErr := models.IsGlobalBannerDismissedForDate(models.RefusedRenewalBannerKey, util.CairoStartOfDay(util.CairoNow())); dismissErr == nil {
			refusedRenewalBannerDismissed = dismissed
		}
	} else {
		log.Printf("ERROR: Failed to count refused renewal due follow-ups: %v", coldErr)
	}

	userRole := middleware.GetUserRole(r)
	data := map[string]interface{}{
		"Title":                    "Pre-Enrolment - Eighty Twenty",
		"Leads":                    leads,
		"UserRole":                 userRole,
		"IsModerator":              IsModerator(r),
		"IsAdmin":                  IsAdmin(r),
		"FlashMessage":             flashMessage,
		"FlashMessageType":         flashMessageType,
		"StatusFilter":             statusFilter,
		"SearchFilter":             searchFilter,
		"PaymentFilter":            paymentFilter,
		"HotFilter":                hotFilter,
		"ReturningFilter":          returningFilter,
		"ColdFilter":               coldFilter,
		"RepeatFilter":             repeatFilter,
		"OpsQueueFilter":           opsQueueFilter,
		"SleepingFilter":           sleepingFilter,
		"SnoozedFilter":            snoozedFilter,
		"IsSleepingLeads":          isSleepingLeads,
		"IsSnoozedLeads":           isSnoozedLeads,
		"IncludeCancelled":         includeCancelled,
		"FollowUpCount":            followUpCount,
		"FollowUpFilter":           followUpFilter,
		"TestedResultsCount":       testedResultsCount,
		"SleepingReminderDueCount": sleepingReminderDueCount,
		"RefusedRenewalReasonTabs": refusedRenewalReasonTabs(),
		"RefusedTemplatesByReason": refusedTemplatesByReason,
		"RefusedRenewalDueCount":   refusedRenewalDueCount,
		"ShowRefusedRenewalBanner": refusedRenewalDueCount > 0 && !refusedRenewalBannerDismissed,
		"ColdLevelOptions":         coldLevelOptions,
		"SelectedColdLevel":        selectedColdLevel,
		"IsWaitingListView":        isWaitingListView,
		"WaitingListBuckets":       waitingListBuckets,
	}
	renderTemplate(w, r, "pre_enrolment_list.html", data)
}

func (h *PreEnrolmentHandler) SendSleepingLeadFollowUp(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "pre-enrolment" || pathParts[2] != "sleeping-follow-up" {
		http.NotFound(w, r)
		return
	}

	leadID, err := uuid.Parse(pathParts[1])
	if err != nil {
		redirectWithError(w, r, "/pre-enrolment?sleeping=1", "Invalid lead for sleeping follow-up.")
		return
	}

	item, err := models.GetSleepingLeadByID(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to load sleeping lead %s: %v", leadID, err)
		redirectWithError(w, r, "/pre-enrolment?sleeping=1", "Couldn't load this sleeping lead right now.")
		return
	}
	if item == nil || item.SleepingLeadStep < 1 || item.SleepingLeadStep > 3 {
		redirectWithError(w, r, "/pre-enrolment?sleeping=1", "This lead is no longer due for a sleeping follow-up.")
		return
	}
	if snooze, err := models.GetLeadSnooze(leadID); err == nil && snooze != nil && util.CairoNow().Before(snooze.SnoozedUntil) {
		redirectWithError(w, r, "/pre-enrolment?snoozed=1", fmt.Sprintf("الحالة دي متأجلة لحد %s.", util.FormatDateCairo(snooze.SnoozedUntil)))
		return
	}

	messageText := buildSleepingLeadMessage(item.Lead.FullName, item.SleepingLeadStep)
	whatsAppURL := buildWhatsAppComposeLink(item.Lead.Phone, messageText)
	if whatsAppURL == "" {
		redirectWithError(w, r, "/pre-enrolment?sleeping=1", "This lead does not have a valid WhatsApp number.")
		return
	}

	userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(r)))
	if err != nil {
		redirectWithError(w, r, "/pre-enrolment?sleeping=1", "Couldn't identify the current user for follow-up logging.")
		return
	}
	if err := models.RecordSleepingLeadFollowUp(leadID, item.SleepingLeadStep, userID); err != nil {
		log.Printf("ERROR: Failed to record sleeping follow-up for lead %s: %v", leadID, err)
		redirectWithError(w, r, "/pre-enrolment?sleeping=1", "Couldn't record this sleeping follow-up.")
		return
	}
	if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
		LeadID:          leadID,
		Channel:         "whatsapp",
		EventType:       "message_ready",
		Source:          "sleeping_lead_sequence",
		TemplateKey:     fmt.Sprintf("sleeping_message_%d", item.SleepingLeadStep),
		MessageText:     messageText,
		Metadata:        map[string]interface{}{"step": item.SleepingLeadStep},
		CreatedByUserID: &userID,
	}); err != nil {
		log.Printf("ERROR: Failed to log sleeping lead contact history for lead %s: %v", leadID, err)
	}

	http.Redirect(w, r, whatsAppURL, http.StatusFound)
}

func (h *PreEnrolmentHandler) SendOfferSentFollowUp(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 || pathParts[0] != "pre-enrolment" || pathParts[2] != "offer-follow-up" {
		http.NotFound(w, r)
		return
	}

	leadID, err := uuid.Parse(pathParts[1])
	if err != nil {
		redirectWithError(w, r, "/pre-enrolment", "Invalid lead for offer follow-up.")
		return
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to load offer follow-up lead %s: %v", leadID, err)
		redirectWithError(w, r, "/pre-enrolment", "Couldn't load this lead right now.")
		return
	}
	if detail == nil || detail.Lead == nil || detail.Lead.Status != "offer_sent" {
		redirectWithError(w, r, "/pre-enrolment", "This lead is no longer in offer follow-up stage.")
		return
	}
	if snooze, err := models.GetLeadSnooze(leadID); err == nil && snooze != nil && util.CairoNow().Before(snooze.SnoozedUntil) {
		redirectWithError(w, r, "/pre-enrolment?snoozed=1", fmt.Sprintf("الحالة دي متأجلة لحد %s.", util.FormatDateCairo(snooze.SnoozedUntil)))
		return
	}

	var amountPaid, finalPrice sql.NullInt32
	if detail.Payment != nil {
		amountPaid = detail.Payment.AmountPaid
	}
	if detail.Offer != nil {
		finalPrice = detail.Offer.FinalPrice
	}
	item := &models.LeadListItem{
		Lead:       detail.Lead,
		AmountPaid: amountPaid,
		FinalPrice: finalPrice,
	}
	lastStep, lastSentAt, followUpErr := models.GetLatestOfferSentFollowUp(leadID)
	if followUpErr != nil {
		log.Printf("ERROR: Failed to load latest offer follow-up for lead %s: %v", leadID, followUpErr)
		redirectWithError(w, r, "/pre-enrolment", "Couldn't load offer follow-up state.")
		return
	}
	item.OfferFollowUpLastStep = lastStep
	item.OfferFollowUpLastSent = lastSentAt
	offerReminder, reminderErr := models.GetOfferSentReminder(leadID)
	if reminderErr != nil {
		log.Printf("ERROR: Failed to load offer reminder for lead %s: %v", leadID, reminderErr)
		redirectWithError(w, r, "/pre-enrolment", "Couldn't load offer reminder state.")
		return
	}
	if offerReminder != nil {
		item.OfferReminderAt = sql.NullTime{Time: offerReminder.FollowUpAt, Valid: true}
		item.OfferReminderNote = offerReminder.Note
	}
	models.ComputeLeadFlags(item)

	if item.PaymentState != models.PaymentStateUnpaid || item.OfferFollowUpStep < 1 || item.OfferFollowUpStep > 3 || !item.OfferFollowUpDueNow {
		redirectWithError(w, r, "/pre-enrolment", "This offer follow-up message is not due right now.")
		return
	}

	messageText := buildOfferSentFollowUpMessage(item.Lead.FullName, item.OfferFollowUpStep)
	whatsAppURL := buildWhatsAppComposeLink(item.Lead.Phone, messageText)
	if whatsAppURL == "" {
		redirectWithError(w, r, "/pre-enrolment", "This lead does not have a valid WhatsApp number.")
		return
	}

	userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(r)))
	if err != nil {
		redirectWithError(w, r, "/pre-enrolment", "Couldn't identify the current user for follow-up logging.")
		return
	}
	if err := models.RecordOfferSentFollowUp(leadID, item.OfferFollowUpStep, userID); err != nil {
		log.Printf("ERROR: Failed to record offer follow-up for lead %s: %v", leadID, err)
		redirectWithError(w, r, "/pre-enrolment", "Couldn't record this offer follow-up.")
		return
	}
	if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
		LeadID:          leadID,
		Channel:         "whatsapp",
		EventType:       "message_ready",
		Source:          "offer_sent_sequence",
		TemplateKey:     fmt.Sprintf("offer_message_%d", item.OfferFollowUpStep),
		MessageText:     messageText,
		Metadata:        map[string]interface{}{"step": item.OfferFollowUpStep},
		CreatedByUserID: &userID,
	}); err != nil {
		log.Printf("ERROR: Failed to log offer follow-up contact history for lead %s: %v", leadID, err)
	}

	http.Redirect(w, r, whatsAppURL, http.StatusFound)
}

func (h *PreEnrolmentHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	h.cfg.Debugf("📝 NewForm() called - rendering pre_enrolment_new.html template")
	data := map[string]interface{}{
		"Title":       "New Lead - Eighty Twenty",
		"UserRole":    middleware.GetUserRole(r),
		"IsModerator": IsModerator(r),
	}
	renderTemplate(w, r, "pre_enrolment_new.html", data)
	h.cfg.Debugf("  → Template render complete")
}

func (h *PreEnrolmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	fullName := r.FormValue("full_name")
	phone := r.FormValue("phone")
	source := r.FormValue("source")
	notes := r.FormValue("notes")

	if fullName == "" || phone == "" {
		data := map[string]interface{}{
			"Title":       "New Lead - Eighty Twenty",
			"Error":       "Full name and phone are required",
			"UserRole":    middleware.GetUserRole(r),
			"IsModerator": IsModerator(r),
		}
		renderTemplate(w, r, "pre_enrolment_new.html", data)
		return
	}

	// Validate source is one of allowed options
	allowedSources := map[string]bool{
		"Facebook": true,
		"WhatsApp": true,
		"Admin":    true,
		"Referral": true,
		"Other":    true,
	}
	if source == "" || !allowedSources[source] {
		source = "Other" // Default to Other if invalid
	}

	userID := middleware.GetUserID(r)
	lead, err := models.CreateLead(fullName, phone, source, notes, userID)
	if err != nil {
		// Check if it's a phone constraint error
		var phoneErr *models.PhoneAlreadyExistsError
		if errors.As(err, &phoneErr) {
			// phoneErr already has the details from CreateLead
		} else if phoneConstraintErr := models.IsPhoneConstraintError(err); phoneConstraintErr != nil {
			// Try to get existing lead
			if existingLead, findErr := models.GetLeadByPhone(phone); findErr == nil {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:          phone,
					ExistingLeadID: &existingLead.ID,
					Message:        "Phone number already exists",
				}
			} else {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:   phone,
					Message: "Phone number already exists",
				}
			}
		}

		if phoneErr != nil {
			data := map[string]interface{}{
				"Title":             "New Lead - Eighty Twenty",
				"Error":             phoneErr.Message,
				"PhoneError":        phoneErr.Message,
				"ExistingLeadID":    phoneErr.ExistingLeadID,
				"PreservedFullName": fullName,
				"PreservedPhone":    phone,
				"PreservedSource":   source,
				"PreservedNotes":    notes,
				"UserRole":          middleware.GetUserRole(r),
				"IsModerator":       IsModerator(r),
			}
			renderTemplate(w, r, "pre_enrolment_new.html", data)
			return
		}

		http.Error(w, "Couldn't create this lead. Please try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?new=1", lead.ID.String()), http.StatusFound)
}

func (h *PreEnrolmentHandler) Detail(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("open_whatsapp") == "1" {
		whatsAppURL := buildWhatsAppComposeLink(detail.Lead.Phone, "")
		if whatsAppURL == "" {
			redirectWithError(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "This lead does not have a valid WhatsApp number.")
			return
		}
		var actorID *uuid.UUID
		if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
			if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
				actorID = &parsed
			}
		}
		if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
			LeadID:          leadID,
			Channel:         "whatsapp",
			EventType:       "chat_opened",
			Source:          "lead_whatsapp",
			CreatedByUserID: actorID,
		}); err != nil {
			log.Printf("ERROR: Failed to log manual WhatsApp open for lead %s: %v", leadID, err)
		}
		http.Redirect(w, r, whatsAppURL, http.StatusFound)
		return
	}

	userRole := middleware.GetUserRole(r)

	if userRole == "admin" && detail.Lead.Status == "paid_full" {
		needsSchedule := detail.Scheduling == nil || !detail.Scheduling.ClassDays.Valid || !detail.Scheduling.ClassTime.Valid
		if needsSchedule {
			classDays, classTime, classKey, ok, err := models.GetLatestEnrollmentSchedule(leadID)
			if err != nil {
				log.Printf("WARNING: Failed to load latest enrollment schedule for lead %s: %v", leadID, err)
			} else if ok {
				if detail.Scheduling == nil {
					detail.Scheduling = &models.Scheduling{LeadID: leadID}
				}
				if !detail.Scheduling.ClassDays.Valid {
					detail.Scheduling.ClassDays = sql.NullString{String: classDays, Valid: true}
				}
				if !detail.Scheduling.ClassTime.Valid {
					detail.Scheduling.ClassTime = sql.NullString{String: classTime, Valid: true}
				}
				if !detail.Scheduling.ClassGroupIndex.Valid && classKey != "" {
					parts := strings.Split(classKey, "|")
					if len(parts) >= 4 {
						if idx, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
							detail.Scheduling.ClassGroupIndex = sql.NullInt32{Int32: int32(idx), Valid: true}
						}
					}
				}
			}
		}
	}

	data, _ := h.buildDetailViewModel(detail, leadID, userRole)
	isReadOnly, _ := data["IsReadOnly"].(bool)
	if r.URL.Query().Get("source") == "finance" && detail.Lead.SentToClasses {
		isReadOnly = true
		data["ReadOnlyReason"] = "Lead already sent to classes."
	}
	data["IsReadOnly"] = isReadOnly
	data["ReturnToListAfterSave"] = r.URL.Query().Get("new") == "1"

	errorMsg := ""
	errorCode := ""
	phoneError := ""
	var existingLeadID *uuid.UUID
	switch r.URL.Query().Get("error") {
	case "future_date":
		errorCode = "future_date"
		errorMsg = "Refund date cannot be in the future"
	case "refund_required":
		errorCode = "refund_required"
		errorMsg = "Refund amount is required when cancelling a lead with course payments"
	case "invalid_amount":
		errorCode = "invalid_amount"
		errorMsg = "Invalid refund amount. Amount must be greater than 0"
	case "amount_exceeds":
		errorCode = "amount_exceeds"
		if maxStr := r.URL.Query().Get("max"); maxStr != "" {
			errorMsg = fmt.Sprintf("Refund amount cannot exceed total course paid (%s EGP)", maxStr)
		} else {
			errorMsg = "Refund amount cannot exceed total course paid"
		}
	case "method_required":
		errorCode = "method_required"
		errorMsg = "Refund payment method is required when cancelling a lead with course payments"
	case "invalid_method":
		errorCode = "invalid_method"
		errorMsg = "Invalid refund payment method"
	case "date_required":
		errorCode = "date_required"
		errorMsg = "Refund date is required when cancelling a lead with course payments"
	case "invalid_date":
		errorCode = "invalid_date"
		errorMsg = "Invalid refund date format. Please use YYYY-MM-DD format"
	case "refund_failed":
		errorCode = "refund_failed"
		errorMsg = "Failed to create refund. Please try again"
	case "phone_exists":
		errorCode = "phone_exists"
		errorMsg = "Phone number already exists"
		phoneError = "Phone number already exists"
		if existingIDStr := r.URL.Query().Get("existing_lead_id"); existingIDStr != "" {
			if parsedID, err := uuid.Parse(existingIDStr); err == nil {
				existingLeadID = &parsedID
			}
		}
	}
	if errorMsg == "" {
		if rawErr := r.URL.Query().Get("error"); rawErr != "" {
			errorCode = rawErr
			errorMsg = rawErr
		}
	}
	data["Error"] = errorMsg
	data["ErrorCode"] = errorCode
	data["PhoneError"] = phoneError
	data["ExistingLeadID"] = existingLeadID

	successMsg := ""
	flashMessage, flashMessageType := flashFromQuery(r)
	if r.URL.Query().Get("cancelled") == "1" && r.URL.Query().Get("refund_recorded") == "1" {
		successMsg = "Lead cancelled and refund recorded."
	} else if r.URL.Query().Get("cancelled") == "1" {
		successMsg = "Lead cancelled successfully."
	} else if r.URL.Query().Get("landing_contacted") == "1" {
		successMsg = "Landing lead marked as contacted."
	} else if r.URL.Query().Get("saved") == "1" {
		successMsg = "Lead saved successfully!"
	} else if flashMessageType == "success" {
		successMsg = flashMessage
	}
	data["SuccessMessage"] = successMsg
	if data["Error"] == "" && flashMessageType == "error" {
		data["Error"] = flashMessage
	}

	actionMode := r.URL.Query().Get("action")
	showCancelModal := actionMode == "cancel" || actionMode == "refund_review"
	data["ShowCancelModal"] = showCancelModal
	data["CancelFlowMode"] = actionMode

	isFullyPaid := data["IsFullyPaid"].(bool)
	if isFullyPaid &&
		detail.Lead.Status != "paid_full" &&
		detail.Lead.Status != "cancelled" &&
		!isPaidWaitingFlowStatus(detail.Lead.Status) {
		_ = models.UpdateLeadStatusFromPayment(leadID)
	}

	if showCancelModal {
		var placementTestPaid int32 = 0
		if detail.PlacementTest != nil && detail.PlacementTest.PlacementTestFeePaid.Valid {
			placementTestPaid = detail.PlacementTest.PlacementTestFeePaid.Int32
		}
		data["PlacementTestPaid"] = placementTestPaid

		// Check if student has remaining credits (covers IsReturning, renewal_pending, waiting_for_round)
		hasRemainingCredits := computedRemainingCredits(detail.Lead) > 0

		// Calculate course paid total (current cycle for students with remaining credits)
		var totalCoursePaid int32
		var err error
		if hasRemainingCredits || detail.Lead.IsReturning {
			totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
		} else {
			totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
		}
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			totalCoursePaid = 0
		}
		data["TotalCoursePaid"] = totalCoursePaid

		// Calculate unused credits value for students with remaining credits
		var unusedCreditsValue int32 = 0
		var calculatedRemainingCredits int32 = 0
		var consumedLevelsForRefund int32 = 0
		var consumedValueForRefund int32 = 0
		var originalPaidForRefund int32 = 0
		if hasRemainingCredits {
			// Calculate dynamic remaining credits for display
			if detail.Lead.LevelsPurchasedTotal.Valid && detail.Lead.LevelsConsumed.Valid {
				calculatedRemainingCredits = detail.Lead.LevelsPurchasedTotal.Int32 - detail.Lead.LevelsConsumed.Int32
				if calculatedRemainingCredits < 0 {
					calculatedRemainingCredits = 0
				}
			}
			breakdown, refundErr := models.GetUnusedCreditsRefundBreakdown(leadID)
			if refundErr != nil {
				log.Printf("ERROR: Failed to calculate unused credits refund: %v", refundErr)
				h.renderDetailWithError(w, r, leadID, "System error: Cannot calculate unused credits refund safely. Please contact support.")
				return
			}
			unusedCreditsValue = breakdown.UnusedCreditsValue
			if breakdown.RemainingCredits > 0 {
				calculatedRemainingCredits = breakdown.RemainingCredits
			}
			consumedLevelsForRefund = breakdown.ConsumedLevels
			consumedValueForRefund = breakdown.ConsumedValue
			originalPaidForRefund = breakdown.OriginalPaidValue

		}
		data["UnusedCreditsValue"] = unusedCreditsValue
		data["RemainingCreditsCount"] = calculatedRemainingCredits
		data["ConsumedLevelsForRefund"] = consumedLevelsForRefund
		data["ConsumedValueForRefund"] = consumedValueForRefund
		data["OriginalPaidForRefund"] = originalPaidForRefund

		// Total refundable amount uses unused-credits valuation when present (no double count).
		totalRefundableAmount := computeCancelRefundableAmount(totalCoursePaid, unusedCreditsValue)
		data["TotalRefundableAmount"] = totalRefundableAmount

		// Get offer final price for remaining balance calculation
		// Use -1 to indicate "not applicable" when FinalPrice is not set
		// (0 means "paid in full", which is different from "price not set yet")
		var remainingBalance int32 = -1
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			remainingBalance = detail.Offer.FinalPrice.Int32 - totalCoursePaid
			if remainingBalance < 0 {
				remainingBalance = 0
			}
		}
		data["RemainingBalance"] = remainingBalance

		// Get lead payments for display
		leadPayments, err := models.GetLeadPayments(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get lead payments: %v", err)
			leadPayments = []*models.LeadPayment{}
		}
		data["LeadPayments"] = leadPayments

		// Calculate final price
		var finalPriceValue int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			finalPriceValue = detail.Offer.FinalPrice.Int32
		}
		data["FinalPrice"] = finalPriceValue

		today := time.Now().Format("2006-01-02")
		data["Today"] = today
	}
	renderTemplate(w, r, "pre_enrolment_detail.html", data)
}

// buildDetailViewModel returns the shared detail page data map used by both Detail() and renderDetailWithError.
// Callers merge overrides (Error, SuccessMessage, ShowCancelModal, PhoneError, ExistingLeadID, PlacementTestPaid).
func (h *PreEnrolmentHandler) buildDetailViewModel(detail *models.LeadDetail, leadID uuid.UUID, userRole string) (map[string]interface{}, error) {
	var placementTestRemaining int32
	if detail.PlacementTest != nil {
		normalizeLegacyPlacementTestFee(detail.PlacementTest)
		feeValue := int32(60)
		if detail.PlacementTest.PlacementTestFee.Valid {
			feeValue = detail.PlacementTest.PlacementTestFee.Int32
		}
		finalFee := computePlacementTestFinalFee(feeValue, detail.PlacementTest.DiscountValue, detail.PlacementTest.DiscountType)
		paidValue := int32(0)
		if detail.PlacementTest.PlacementTestFeePaid.Valid {
			paidValue = detail.PlacementTest.PlacementTestFeePaid.Int32
		}
		placementTestRemaining = finalFee - paidValue
		if placementTestRemaining < 0 {
			placementTestRemaining = 0
		}
	} else {
		placementTestRemaining = 60
	}
	var amountPaid, finalPrice sql.NullInt32
	if detail.Payment != nil {
		amountPaid = detail.Payment.AmountPaid
	}
	if detail.Offer != nil {
		finalPrice = detail.Offer.FinalPrice
	}
	var testDate sql.NullTime
	if detail.PlacementTest != nil {
		testDate = detail.PlacementTest.TestDate
	}
	tempItem := &models.LeadListItem{
		Lead:       detail.Lead,
		TestDate:   testDate,
		AmountPaid: amountPaid,
		FinalPrice: finalPrice,
	}
	lastOfferStep, lastOfferSentAt, offerFollowUpErr := models.GetLatestOfferSentFollowUp(leadID)
	if offerFollowUpErr != nil {
		log.Printf("ERROR: Failed to get offer follow-up state: %v", offerFollowUpErr)
	}
	tempItem.OfferFollowUpLastStep = lastOfferStep
	tempItem.OfferFollowUpLastSent = lastOfferSentAt
	offerReminder, offerReminderErr := models.GetOfferSentReminder(leadID)
	if offerReminderErr != nil {
		log.Printf("ERROR: Failed to get offer reminder: %v", offerReminderErr)
	}
	offerReminderDue := false
	offerReminderDate := ""
	offerReminderNote := ""
	if offerReminder != nil {
		tempItem.OfferReminderAt = sql.NullTime{Time: offerReminder.FollowUpAt, Valid: true}
		tempItem.OfferReminderNote = offerReminder.Note
		offerReminderDue = !util.CairoNow().Before(offerReminder.FollowUpAt)
		offerReminderDate = util.FormatDateCairo(offerReminder.FollowUpAt)
		if offerReminder.Note.Valid {
			offerReminderNote = offerReminder.Note.String
		}
	}
	models.ComputeLeadFlags(tempItem)
	tempItem.WhatsAppURL = buildLeadWhatsAppURL(tempItem)
	sleepingLeadItem, sleepingLeadErr := models.GetSleepingLeadByID(leadID)
	if sleepingLeadErr != nil {
		log.Printf("ERROR: Failed to get sleeping lead state: %v", sleepingLeadErr)
		sleepingLeadItem = nil
	}
	sleepingReminder, reminderErr := models.GetSleepingLeadReminder(leadID)
	if reminderErr != nil {
		log.Printf("ERROR: Failed to get sleeping lead reminder: %v", reminderErr)
		sleepingReminder = nil
	}
	sleepingReminderDue := false
	sleepingReminderDate := ""
	sleepingReminderNote := ""
	if sleepingReminder != nil {
		sleepingReminderDue = !util.CairoNow().Before(sleepingReminder.FollowUpAt)
		sleepingReminderDate = util.FormatDateCairo(sleepingReminder.FollowUpAt)
		if sleepingReminder.Note.Valid {
			sleepingReminderNote = sleepingReminder.Note.String
		}
	}
	leadSnooze, leadSnoozeErr := models.GetLeadSnooze(leadID)
	if leadSnoozeErr != nil {
		log.Printf("ERROR: Failed to get lead snooze: %v", leadSnoozeErr)
		leadSnooze = nil
	}
	leadSnoozeDue := false
	leadSnoozeDate := ""
	leadSnoozeNote := ""
	if leadSnooze != nil {
		leadSnoozeDue = !util.CairoNow().Before(leadSnooze.SnoozedUntil)
		leadSnoozeDate = util.FormatDateCairo(leadSnooze.SnoozedUntil)
		if leadSnooze.Note.Valid {
			leadSnoozeNote = leadSnooze.Note.String
		}
		if leadSnoozeDue {
			tempItem.NextAction = "التذكير مستحق دلوقتي"
			tempItem.FollowUpDue = true
		}
	}

	today := time.Now().Format("2006-01-02")
	leadPayments := []*models.LeadPayment{}
	previousPayments := []*models.LeadPayment{}
	unidentifiedTransfers := []*models.Transaction{}
	var cycleStart *time.Time
	var latestEnrollment *models.ClassEnrollment
	if latest, err := models.GetLatestClassEnrollment(leadID); err == nil {
		latestEnrollment = latest
	} else {
		log.Printf("ERROR: Failed to get latest class enrollment: %v", err)
	}
	if detail.Lead.IsReturning {
		if cs, err := models.GetLatestClassOutcome(leadID); err == nil && cs != nil && cs.CompletedAt.Valid {
			cycleStart = &cs.CompletedAt.Time
		}
	}
	if cycleStart != nil {
		if current, err := models.GetLeadPaymentsSince(leadID, *cycleStart); err == nil {
			leadPayments = current
		} else {
			log.Printf("ERROR: Failed to get current-cycle payments: %v", err)
		}
		if prev, err := models.GetLeadPaymentsBefore(leadID, *cycleStart); err == nil {
			previousPayments = prev
		} else {
			log.Printf("ERROR: Failed to get previous payments: %v", err)
		}

		// Schedule Fallback Logic:
		// For returning students, if their current scheduling preferences are empty,
		// default them to the values from their latest class enrollment.
		if detail.Lead.IsReturning && latestEnrollment != nil && detail.Scheduling != nil && (!detail.Scheduling.ClassDays.Valid || !detail.Scheduling.ClassTime.Valid) {
			if !detail.Scheduling.ClassDays.Valid && latestEnrollment.ClassDays != "" {
				detail.Scheduling.ClassDays = sql.NullString{String: latestEnrollment.ClassDays, Valid: true}
			}
			if !detail.Scheduling.ClassTime.Valid && latestEnrollment.ClassTime != "" {
				timeStr := latestEnrollment.ClassTime
				if len(timeStr) > 5 {
					timeStr = timeStr[:5]
				}
				detail.Scheduling.ClassTime = sql.NullString{String: timeStr, Valid: true}
			}
		}
	} else {
		lp, err := models.GetLeadPayments(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get lead payments: %v", err)
		} else {
			leadPayments = lp
		}
	}
	if userRole == "admin" || userRole == "manager" {
		if transfers, err := models.GetUnidentifiedTransfers(); err == nil {
			unidentifiedTransfers = transfers
		} else {
			log.Printf("ERROR: Failed to load unidentified transfers: %v", err)
		}
	}

	var finalPriceValue int32 = 0
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
		finalPriceValue = detail.Offer.FinalPrice.Int32
	}
	var err error
	totalCoursePaid := int32(0)
	if detail.Lead.IsReturning {
		totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
	} else {
		totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
	}
	if err != nil {
		log.Printf("ERROR: Failed to get total course paid: %v", err)
		totalCoursePaid = 0
	}
	// Get offer final price for remaining balance calculation
	// Use -1 to indicate "not applicable" when FinalPrice is not set
	// (0 means "paid in full", which is different from "price not set yet")
	var remainingBalance int32 = -1
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
		remainingBalance = finalPriceValue - totalCoursePaid
		if remainingBalance < 0 {
			remainingBalance = 0
		}
	}
	// For returning students:
	// - waiting_for_round: Already paid via previous bundle, no payment needed
	// - renewal_pending: Need to pay for new offer
	hasCredits := computedRemainingCredits(detail.Lead) > 0
	isFullyPaid := (detail.Offer != nil && detail.Offer.FinalPrice.Valid && totalCoursePaid >= finalPriceValue) || hasCredits

	pipelineStatuses := map[string]bool{
		"lead_created": true, "test_booked": true, "tested": true, "offer_sent": true,
	}
	showFollowUpBanner := !isFullyPaid && tempItem.FollowUpDue && pipelineStatuses[detail.Lead.Status]

	statusInfo := models.GetStatusDisplayInfo(detail.Lead.Status)
	if isFullyPaid {
		statusInfo = models.GetStatusDisplayInfo("paid_full")
	}

	var lateJoiner *models.LateJoiner
	var classCurrentSession int32 = 0
	lj, err := models.GetLateJoinerByLeadID(leadID)
	if err == nil && lj != nil {
		lateJoiner = lj
		// Get current session for the class they joined
		if sess, err := models.GetActiveClassCurrentSession(lj.ClassKey); err == nil {
			classCurrentSession = sess
		}
	}

	coldAnchor := detail.Lead.UpdatedAt
	if detail.Lead.OfferSentAt.Valid {
		coldAnchor = detail.Lead.OfferSentAt.Time
	}
	coldEligible := detail.Lead.Status == "offer_sent" && offerReminder == nil && time.Since(coldAnchor) >= 7*24*time.Hour

	var continuationHoldCandidate *models.ClassEnrollment
	if candidate, err := models.GetLatestContinuationHoldCandidate(leadID); err == nil {
		continuationHoldCandidate = candidate
	} else {
		log.Printf("ERROR: Failed to get continuation hold candidate: %v", err)
	}

	canApplyContinuationHold := (userRole == "admin" || userRole == "manager") &&
		detail.Lead.Status == "waiting_for_round" &&
		continuationHoldCandidate != nil &&
		!continuationHoldCandidate.ContinuationHoldActive
	canReleaseContinuationHold := (userRole == "admin" || userRole == "manager") &&
		detail.Lead.Status == "paused" &&
		continuationHoldCandidate != nil &&
		continuationHoldCandidate.ContinuationHoldActive
	continuationHoldReason := ""
	if continuationHoldCandidate != nil && continuationHoldCandidate.ContinuationHoldReason.Valid {
		continuationHoldReason = continuationHoldCandidate.ContinuationHoldReason.String
	}
	canAddBundleCredit := (userRole == "admin" || userRole == "manager") &&
		detail.Lead.Status == "waiting_for_round"

	creditsRemaining := int32(0)
	if detail.Lead.LevelsPurchasedTotal.Valid {
		creditsRemaining = detail.Lead.LevelsPurchasedTotal.Int32
	}
	if detail.Lead.LevelsConsumed.Valid {
		creditsRemaining -= detail.Lead.LevelsConsumed.Int32
	}
	if creditsRemaining < 0 {
		creditsRemaining = 0
	}
	hasCarryoverCredits := creditsRemaining > 0

	canApplyContinuationHold = canApplyContinuationHold && hasCarryoverCredits

	lastOutcome, lastGrade := "", ""
	if latest, err := models.GetLatestClassOutcome(leadID); err == nil && latest != nil {
		if latest.Outcome.Valid {
			lastOutcome = latest.Outcome.String
		}
		if latest.FinalGrade.Valid {
			lastGrade = latest.FinalGrade.String
		}
	}
	var renewalPendingDecision *renewalPendingMessageDecision
	if detail.Lead.Status == "renewal_pending" && detail.Lead.IsReturning && latestEnrollment != nil {
		attendedSessions := 0
		if latestEnrollment.ClassKey != "" {
			if count, err := models.CountPresentedAttendanceForClass(leadID, latestEnrollment.ClassKey); err == nil {
				attendedSessions = count
			} else {
				log.Printf("ERROR: Failed to count presented attendance for lead %s in class %s: %v", leadID, latestEnrollment.ClassKey, err)
			}
		}
		if decision, ok := buildRenewalPendingMessage(detail.Lead.FullName, int(latestEnrollment.Level), lastOutcome, attendedSessions); ok {
			renewalPendingDecision = decision
		}
	}
	latestRefusal, err := models.GetLatestRenewalRefusal(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to get latest renewal refusal: %v", err)
	}
	isRefusedRenewal := latestRefusal != nil
	refusedAtText := ""
	refusedReason := ""
	refusedReasonLabelText := ""
	refusedOtherNote := ""
	refusedFollowUpStep := 0
	refusedFollowUpDueAt := ""
	refusedFollowUpDueNow := false
	refusedFollowUpManual := false
	if latestRefusal != nil {
		refusedAtText = latestRefusal.RefusedAt.Format("2006-01-02")
		refusedReason = latestRefusal.Reason
		refusedReasonLabelText = refusalReasonLabel(latestRefusal.Reason)
		if latestRefusal.Notes.Valid {
			refusedOtherNote = latestRefusal.Notes.String
		}
		lastRefusedStep, lastRefusedSentAt, followUpErr := models.GetLatestRefusedRenewalFollowUp(leadID)
		if followUpErr != nil {
			log.Printf("ERROR: Failed to get refused renewal follow-up state: %v", followUpErr)
		} else {
			var dueAt time.Time
			var dueNow bool
			var manualAvailable bool
			refusedFollowUpStep, dueAt, dueNow, manualAvailable = models.ComputeRefusedRenewalFollowUpState(latestRefusal.RefusedAt, lastRefusedStep, lastRefusedSentAt, util.CairoNow())
			refusedFollowUpDueNow = dueNow
			refusedFollowUpManual = manualAvailable
			if !dueAt.IsZero() {
				refusedFollowUpDueAt = util.FormatDateCairo(dueAt)
			}
		}
	}
	refusedTemplates, err := models.GetRefusedRenewalMessageTemplates()
	if err != nil {
		log.Printf("ERROR: Failed to load refused renewal templates: %v", err)
		refusedTemplates = []*models.RefusedRenewalMessageTemplate{}
	}
	contactHistory, err := models.GetPreEnrolmentContactHistory(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to load pre-enrolment contact history: %v", err)
		contactHistory = []*models.ContactHistoryItem{}
	}
	isLandingLeadValue := isLandingLead(detail.Lead)
	landingLeadContactedActive := isLandingLeadContactedInCurrentStatus(detail.Lead)
	landingLeadContactedAt := ""
	if detail.Lead.LandingPageContactedAt.Valid {
		landingLeadContactedAt = detail.Lead.LandingPageContactedAt.Time.Format("2006-01-02 03:04 PM")
	}
	canMarkRefusedRenewal := userRole == "admin" &&
		detail.Lead.IsReturning &&
		creditsRemaining <= 0 &&
		(detail.Lead.Status == "renewal_pending" || detail.Lead.Status == "offer_sent")

	// Prefill schedule for returning students from latest class_enrollments if schedule is empty.
	if detail.Lead.IsReturning {
		missingDays := detail.Scheduling == nil || !detail.Scheduling.ClassDays.Valid
		missingTime := detail.Scheduling == nil || !detail.Scheduling.ClassTime.Valid
		if missingDays || missingTime {
			classDays, classTime, err := models.GetLatestClassSchedule(leadID)
			if err == nil && (classDays.Valid || classTime.Valid) {
				if detail.Scheduling == nil {
					detail.Scheduling = &models.Scheduling{}
				}
				if missingDays && classDays.Valid {
					detail.Scheduling.ClassDays = classDays
				}
				if missingTime && classTime.Valid {
					detail.Scheduling.ClassTime = sql.NullString{String: normalizeClassTime(classTime.String), Valid: true}
				}
			}
		}
	}

	data := map[string]interface{}{
		"Title":                      fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
		"Detail":                     detail,
		"UserRole":                   userRole,
		"IsModerator":                userRole == "moderator",
		"IsAdmin":                    userRole == "admin",
		"IsReadOnly":                 userRole == "student_success",
		"ColdEligible":               coldEligible,
		"LateJoiner":                 lateJoiner,
		"ClassCurrentSession":        classCurrentSession,
		"PlacementTestRemaining":     placementTestRemaining,
		"FollowUpDue":                tempItem.FollowUpDue,
		"ShowFollowUpBanner":         showFollowUpBanner,
		"HotLevel":                   tempItem.HotLevel,
		"NextAction":                 tempItem.NextAction,
		"LeadWhatsAppURL":            tempItem.WhatsAppURL,
		"LeadWhatsAppOpenURL":        fmt.Sprintf("/pre-enrolment/%s?open_whatsapp=1", leadID.String()),
		"OfferFollowUpStep":          tempItem.OfferFollowUpStep,
		"OfferFollowUpDueAt":         tempItem.OfferFollowUpDueAt,
		"OfferFollowUpDueNow":        tempItem.OfferFollowUpDueNow,
		"OfferFollowUpLastStep":      tempItem.OfferFollowUpLastStep,
		"DaysSinceLastProgress":      tempItem.DaysSinceLastProgress,
		"Today":                      today,
		"LeadPayments":               leadPayments,
		"PreviousLeadPayments":       previousPayments,
		"UnidentifiedTransfers":      unidentifiedTransfers,
		"HasUnidentifiedTransfers":   len(unidentifiedTransfers) > 0,
		"FinalPrice":                 finalPriceValue,
		"FinalPriceSet":              detail.Offer != nil && detail.Offer.FinalPrice.Valid,
		"TotalCoursePaid":            totalCoursePaid,
		"RemainingBalance":           remainingBalance,
		"IsFullyPaid":                isFullyPaid,
		"IsWaitingForRound":          detail.Lead.Status == "waiting_for_round",
		"HasCarryoverCredits":        hasCarryoverCredits,
		"IsPaused":                   detail.Lead.Status == "paused",
		"CanMoveWaiting":             canUseWaitingFlow(detail),
		"CanApplyContinuationHold":   canApplyContinuationHold,
		"CanReleaseContinuationHold": canReleaseContinuationHold,
		"ContinuationHoldReason":     continuationHoldReason,
		"CanAddBundleCredit":         canAddBundleCredit,
		"SleepingLeadStep": func() int {
			if sleepingLeadItem == nil {
				return 0
			}
			return sleepingLeadItem.SleepingLeadStep
		}(),
		"SleepingLeadDueNow": func() bool {
			if sleepingLeadItem == nil {
				return false
			}
			return sleepingLeadItem.SleepingLeadDueNow
		}(),
		"HasRenewalPendingMessage": renewalPendingDecision != nil,
		"RenewalPendingMessageKey": func() string {
			if renewalPendingDecision == nil {
				return ""
			}
			return renewalPendingDecision.Key
		}(),
		"RenewalPendingMessageLabel": func() string {
			if renewalPendingDecision == nil {
				return ""
			}
			return renewalPendingDecision.Label
		}(),
		"RenewalPendingMessageText": func() string {
			if renewalPendingDecision == nil {
				return ""
			}
			return renewalPendingDecision.Text
		}(),
		"RenewalPendingMessageLevel": func() int {
			if renewalPendingDecision == nil {
				return 0
			}
			return renewalPendingDecision.Level
		}(),
		"HasUnifiedMessageBank": (renewalPendingDecision != nil) ||
			(sleepingLeadItem != nil && sleepingLeadItem.SleepingLeadDueNow && sleepingLeadItem.SleepingLeadStep > 0) ||
			(tempItem.OfferFollowUpDueNow && tempItem.OfferFollowUpStep > 0) ||
			(refusedFollowUpDueNow && refusedFollowUpStep > 0),
		"MessageBankDefaultCategory": func() string {
			switch {
			case renewalPendingDecision != nil:
				return "renewal_pending"
			case refusedFollowUpDueNow && refusedFollowUpStep > 0:
				return "refused_renewal"
			case sleepingLeadItem != nil && sleepingLeadItem.SleepingLeadDueNow && sleepingLeadItem.SleepingLeadStep > 0:
				return "sleeping_leads"
			case tempItem.OfferFollowUpDueNow && tempItem.OfferFollowUpStep > 0:
				return "after_placement_test"
			default:
				return ""
			}
		}(),
		"CoursePaymentEnabled": func() bool {
			ok, _ := canUseCoursePaymentFlow(detail)
			return ok
		}(),
		"CoursePaymentDisabled": func() bool {
			ok, _ := canUseCoursePaymentFlow(detail)
			return !ok || remainingBalance == 0
		}(),
		"CoursePaymentLockedReason": func() string {
			_, reason := canUseCoursePaymentFlow(detail)
			return reason
		}(),
		"PricingTrack":                inferOfferPricingTrack(detail.Offer),
		"IsPrivateTrack":              detail.Lead.OpsQueueReason.Valid && detail.Lead.OpsQueueReason.String == "private_track",
		"IsRefundReview":              isRefundReviewLead(detail.Lead),
		"IsLandingLead":               isLandingLeadValue,
		"CanMarkLandingLeadContacted": isLandingLeadValue && !landingLeadContactedActive && (userRole == "admin" || userRole == "manager"),
		"LandingLeadContactedActive":  landingLeadContactedActive,
		"LandingLeadContactedAt":      landingLeadContactedAt,
		"LandingLeadContactedByName":  detail.Lead.LandingPageContactedByName,
		"CanMarkOfferSent": func() bool {
			ok, _ := canMarkOfferSent(detail)
			return ok
		}(),
		"OfferActionLockedReason": func() string {
			_, reason := canMarkOfferSent(detail)
			return reason
		}(),
		"StatusDisplayName":         statusInfo.DisplayName,
		"StatusBgColor":             statusInfo.BgColor,
		"StatusTextColor":           statusInfo.TextColor,
		"StatusBorderColor":         statusInfo.BorderColor,
		"CreditsRemaining":          creditsRemaining,
		"LastOutcome":               lastOutcome,
		"LastFinalGrade":            lastGrade,
		"SmartStepsCodes":           []string{},
		"SmartStepsAR":              []string{},
		"SmartStepsSource":          "",
		"IsRefusedRenewal":          isRefusedRenewal,
		"RenewalRefusedAt":          refusedAtText,
		"RenewalRefusedReason":      refusedReason,
		"RenewalRefusedReasonLabel": refusedReasonLabelText,
		"RenewalRefusedOtherNote":   refusedOtherNote,
		"CanMarkRefusedRenewal":     canMarkRefusedRenewal,
		"RefusedFollowUpStep":       refusedFollowUpStep,
		"RefusedFollowUpDueAt":      refusedFollowUpDueAt,
		"RefusedFollowUpDueNow":     refusedFollowUpDueNow,
		"RefusedFollowUpManual":     refusedFollowUpManual,
		"RefusedRenewalReasonTabs":  refusedRenewalReasonTabs(),
		"RefusedTemplatesByReason":  groupRefusedRenewalTemplatesByReason(refusedTemplates),
		"ContactHistory":            contactHistory,
		"CanSetSleepingReminder":    false,
		"SleepingLeadReminder":      sleepingReminder,
		"SleepingReminderDue":       sleepingReminderDue,
		"SleepingReminderDate":      sleepingReminderDate,
		"SleepingReminderNote":      sleepingReminderNote,
		"CanSetOfferReminder":       false,
		"OfferReminder":             offerReminder,
		"OfferReminderDue":          offerReminderDue,
		"OfferReminderDate":         offerReminderDate,
		"OfferReminderNote":         offerReminderNote,
		"CanSnoozeLead":             canSnoozeLead(detail),
		"LeadSnooze":                leadSnooze,
		"LeadSnoozeDue":             leadSnoozeDue,
		"LeadSnoozeDate":            leadSnoozeDate,
		"LeadSnoozeNote":            leadSnoozeNote,
		"Error":                     "",
		"PhoneError":                "",
		"ExistingLeadID":            nil,
		"SuccessMessage":            "",
		"ShowCancelModal":           false,
		"CoursePaymentInput": map[string]string{
			"source":                   "new_payment",
			"type":                     "",
			"amount":                   "",
			"method":                   "",
			"date":                     "",
			"notes":                    "",
			"unidentified_transfer_id": "",
		},
		"CoursePaymentFieldErrors": map[string]string{},
	}
	stepCodes, stepArabic, stepSource := h.buildSmartStepsForDetail(detail, isFullyPaid, creditsRemaining, finalPriceValue, totalCoursePaid, lastOutcome)
	data["SmartStepsCodes"] = stepCodes
	data["SmartStepsAR"] = stepArabic
	data["SmartStepsSource"] = stepSource

	return data, nil
}

func isLandingLead(lead *models.Lead) bool {
	if lead == nil {
		return false
	}
	if lead.Source.Valid && strings.EqualFold(strings.TrimSpace(lead.Source.String), "Landing Page") {
		return true
	}
	if !lead.Notes.Valid {
		return false
	}
	notes := strings.ToLower(strings.TrimSpace(lead.Notes.String))
	return strings.Contains(notes, "landing page signup") ||
		strings.Contains(notes, "تم التواصل عن طريق السيستم")
}

func isLandingLeadContactedInCurrentStatus(lead *models.Lead) bool {
	if lead == nil {
		return false
	}
	if !lead.LandingPageContactedAt.Valid || !lead.LandingPageContactedStatus.Valid {
		return false
	}
	return strings.TrimSpace(lead.LandingPageContactedStatus.String) == strings.TrimSpace(lead.Status)
}

// renderDetailWithError fetches the lead, builds detail page data with Error set, and renders.
// Uses buildDetailViewModel so template context matches Detail() (status, banners, modal flags, etc.).
func (h *PreEnrolmentHandler) renderDetailWithError(w http.ResponseWriter, r *http.Request, leadID uuid.UUID, errMsg string) {
	h.renderDetailWithErrorAndPaymentContext(w, r, leadID, errMsg, nil, nil)
}

func (h *PreEnrolmentHandler) renderDetailWithErrorAndPaymentContext(
	w http.ResponseWriter,
	r *http.Request,
	leadID uuid.UUID,
	errMsg string,
	coursePaymentInput map[string]string,
	coursePaymentFieldErrors map[string]string,
) {
	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
		return
	}
	applyPlacementTestFormValuesForRender(detail, r)
	userRole := middleware.GetUserRole(r)
	data, _ := h.buildDetailViewModel(detail, leadID, userRole)
	data["Error"] = errMsg
	data["SuccessMessage"] = ""
	data["ReturnToListAfterSave"] = r.URL.Query().Get("new") == "1" || r.FormValue("return_to_list") == "1"
	if coursePaymentInput != nil {
		data["CoursePaymentInput"] = coursePaymentInput
	}
	if coursePaymentFieldErrors != nil {
		data["CoursePaymentFieldErrors"] = coursePaymentFieldErrors
	}
	renderTemplate(w, r, "pre_enrolment_detail.html", data)
}

func normalizeClassTime(raw string) string {
	// Expected formats: "HH:MM" or "HH:MM:SS"
	if len(raw) >= 5 {
		return raw[:5]
	}
	return raw
}

func clampInt32(v, min, max int32) int32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func computePlacementTestFinalFee(baseFee int32, discountValue sql.NullInt32, discountType sql.NullString) int32 {
	fee := baseFee
	if fee <= 0 {
		return 0
	}
	discount := int32(0)
	if discountValue.Valid {
		if discountType.Valid && strings.EqualFold(discountType.String, "percent") {
			pct := clampInt32(discountValue.Int32, 0, 100)
			discount = (fee * pct) / 100
		} else {
			discount = clampInt32(discountValue.Int32, 0, fee)
		}
	}
	finalFee := fee - discount
	if finalFee < 0 {
		return 0
	}
	return finalFee
}

func normalizeLegacyPlacementTestFee(pt *models.PlacementTest) {
	if pt == nil {
		return
	}
	if !pt.PlacementTestFee.Valid || pt.PlacementTestFee.Int32 <= 0 {
		pt.PlacementTestFee = sql.NullInt32{Int32: 60, Valid: true}
		return
	}
	paidAmount := int32(0)
	if pt.PlacementTestFeePaid.Valid {
		paidAmount = pt.PlacementTestFeePaid.Int32
	}
	if paidAmount == 0 && pt.PlacementTestFee.Int32 == 100 {
		pt.PlacementTestFee = sql.NullInt32{Int32: 60, Valid: true}
	}
}

func applyPlacementTestFormValuesForRender(detail *models.LeadDetail, r *http.Request) {
	if detail == nil || detail.Lead == nil || r == nil {
		return
	}
	if r.Method != http.MethodPost {
		return
	}
	if detail.PlacementTest == nil {
		detail.PlacementTest = &models.PlacementTest{LeadID: detail.Lead.ID}
	}
	pt := detail.PlacementTest

	if feeStr := strings.TrimSpace(r.FormValue("placement_test_fee")); feeStr != "" {
		if fee, err := strconv.Atoi(feeStr); err == nil {
			pt.PlacementTestFee = sql.NullInt32{Int32: int32(fee), Valid: true}
		}
	}
	if paidStr := strings.TrimSpace(r.FormValue("placement_test_fee_paid")); paidStr != "" {
		if paid, err := strconv.Atoi(paidStr); err == nil {
			pt.PlacementTestFeePaid = sql.NullInt32{Int32: int32(paid), Valid: true}
		}
	}
	if paymentDateStr := strings.TrimSpace(r.FormValue("placement_test_payment_date")); paymentDateStr != "" {
		if t, err := time.Parse("2006-01-02", paymentDateStr); err == nil {
			pt.PlacementTestPaymentDate = sql.NullTime{Time: t, Valid: true}
		}
	}
	if paymentMethod := strings.TrimSpace(r.FormValue("placement_test_payment_method")); paymentMethod != "" {
		pt.PlacementTestPaymentMethod = sql.NullString{String: paymentMethod, Valid: true}
	}
	if discountValueStr := strings.TrimSpace(r.FormValue("placement_test_discount_value")); discountValueStr != "" {
		if dv, err := strconv.Atoi(discountValueStr); err == nil {
			pt.DiscountValue = sql.NullInt32{Int32: int32(dv), Valid: true}
		}
	}
	if discountType := strings.TrimSpace(r.FormValue("placement_test_discount_type")); discountType == "amount" || discountType == "percent" {
		pt.DiscountType = sql.NullString{String: discountType, Valid: true}
	}
	if testDate := strings.TrimSpace(r.FormValue("test_date")); testDate != "" {
		if t, err := time.Parse("2006-01-02", testDate); err == nil {
			pt.TestDate = sql.NullTime{Time: t, Valid: true}
		}
	}
	if testTime := strings.TrimSpace(r.FormValue("test_time")); testTime != "" {
		pt.TestTime = sql.NullString{String: testTime, Valid: true}
	}
	if testType := strings.TrimSpace(r.FormValue("test_type")); testType != "" {
		pt.TestType = sql.NullString{String: testType, Valid: true}
	}
}

func buildBookedPlacementTestFromRequest(leadID uuid.UUID, existing *models.PlacementTest, r *http.Request) (*models.PlacementTest, error) {
	testDate := strings.TrimSpace(r.FormValue("test_date"))
	testTime := strings.TrimSpace(r.FormValue("test_time"))
	testType := strings.TrimSpace(r.FormValue("test_type"))
	if testDate == "" || testTime == "" || testType == "" {
		return nil, fmt.Errorf("Please choose the test date, time, and type.")
	}

	baseFee := int32(60)
	paidAmount := int32(0)
	discountValue := sql.NullInt32{}
	discountType := sql.NullString{}
	if existing != nil {
		if existing.PlacementTestFee.Valid {
			baseFee = existing.PlacementTestFee.Int32
		}
		if existing.PlacementTestFeePaid.Valid {
			paidAmount = existing.PlacementTestFeePaid.Int32
		}
		discountValue = existing.DiscountValue
		discountType = existing.DiscountType
	}
	if feeStr := strings.TrimSpace(r.FormValue("placement_test_fee")); feeStr != "" {
		if fee, err := strconv.Atoi(feeStr); err == nil {
			baseFee = int32(fee)
		}
	}
	if paidStr := strings.TrimSpace(r.FormValue("placement_test_fee_paid")); paidStr != "" {
		if paid, err := strconv.Atoi(paidStr); err == nil {
			paidAmount = int32(paid)
		}
	}
	if discountValueStr := strings.TrimSpace(r.FormValue("placement_test_discount_value")); discountValueStr != "" {
		if dv, err := strconv.Atoi(discountValueStr); err == nil {
			discountValue = sql.NullInt32{Int32: int32(dv), Valid: true}
		}
	}
	if discountTypeStr := strings.TrimSpace(r.FormValue("placement_test_discount_type")); discountTypeStr == "amount" || discountTypeStr == "percent" {
		discountType = sql.NullString{String: discountTypeStr, Valid: true}
	}

	requiredFee := computePlacementTestFinalFee(baseFee, discountValue, discountType)
	if requiredFee > 0 {
		if paidAmount <= 0 {
			return nil, fmt.Errorf("Paid amount is required before booking the placement test.")
		}
		if paidAmount != requiredFee {
			return nil, fmt.Errorf("Paid amount must equal the final placement test fee (%d EGP) before booking the test.", requiredFee)
		}
	}

	pt := &models.PlacementTest{
		LeadID:               leadID,
		PlacementTestFee:     sql.NullInt32{Int32: baseFee, Valid: true},
		PlacementTestFeePaid: sql.NullInt32{Int32: paidAmount, Valid: true},
	}
	if t, err := time.Parse("2006-01-02", testDate); err == nil {
		pt.TestDate = sql.NullTime{Time: t, Valid: true}
	}
	if testTime != "" {
		pt.TestTime = sql.NullString{String: testTime, Valid: true}
	}
	if testType != "" {
		pt.TestType = sql.NullString{String: testType, Valid: true}
	}
	if notes := strings.TrimSpace(r.FormValue("test_notes")); notes != "" {
		pt.TestNotes = sql.NullString{String: notes, Valid: true}
	}

	if paidAmount > 0 {
		paymentDateStr := strings.TrimSpace(r.FormValue("placement_test_payment_date"))
		switch {
		case paymentDateStr != "":
			t, err := util.ParseDateLocal(paymentDateStr)
			if err != nil {
				return nil, fmt.Errorf("Invalid payment date for placement test.")
			}
			if err := util.ValidateNotFutureDate(t); err != nil {
				return nil, fmt.Errorf("Payment date cannot be in the future")
			}
			pt.PlacementTestPaymentDate = sql.NullTime{Time: t, Valid: true}
		case existing != nil && existing.PlacementTestPaymentDate.Valid:
			pt.PlacementTestPaymentDate = existing.PlacementTestPaymentDate
		default:
			return nil, fmt.Errorf("Payment date is required when placement test fee is paid.")
		}

		paymentMethod := strings.TrimSpace(r.FormValue("placement_test_payment_method"))
		switch {
		case paymentMethod != "":
			pt.PlacementTestPaymentMethod = sql.NullString{String: paymentMethod, Valid: true}
		case existing != nil && existing.PlacementTestPaymentMethod.Valid:
			pt.PlacementTestPaymentMethod = existing.PlacementTestPaymentMethod
		default:
			return nil, fmt.Errorf("Payment method is required when placement test fee is paid.")
		}
	}

	return pt, nil
}

func syncPlacementTestFinanceForBooking(leadID uuid.UUID, placementTest *models.PlacementTest) error {
	if placementTest == nil {
		return nil
	}
	amountPaid := int32(0)
	if placementTest.PlacementTestFeePaid.Valid {
		amountPaid = placementTest.PlacementTestFeePaid.Int32
	}
	return models.UpsertPlacementTestIncome(leadID, amountPaid, placementTest.PlacementTestPaymentDate, placementTest.PlacementTestPaymentMethod)
}

func computedRemainingCredits(lead *models.Lead) int32 {
	if lead == nil {
		return 0
	}
	if lead.LevelsPurchasedTotal.Valid {
		remaining := lead.LevelsPurchasedTotal.Int32
		if lead.LevelsConsumed.Valid {
			remaining -= lead.LevelsConsumed.Int32
		}
		if remaining < 0 {
			remaining = 0
		}
		return remaining
	}
	if lead.RemainingCredits.Valid && lead.RemainingCredits.Int32 > 0 {
		return lead.RemainingCredits.Int32
	}
	return 0
}

func computeCancelRefundableAmount(totalCoursePaid, unusedCreditsValue int32) int32 {
	// Unused-credits value is already a final refundable amount for carryover-credit cases.
	// Adding it to current-cycle paid would double count.
	if unusedCreditsValue > 0 {
		return unusedCreditsValue
	}
	return totalCoursePaid
}

func canUseWaitingFlow(detail *models.LeadDetail) bool {
	if detail == nil || detail.Lead == nil {
		return false
	}
	switch detail.Lead.Status {
	case "cancelled", "in_classes", "cold_lead":
		return false
	}

	hasCredits := computedRemainingCredits(detail.Lead) > 0
	alreadyInWaitingFlow := detail.Lead.Status == "waiting_for_round" || detail.Lead.Status == "schedule_assigned" || detail.Lead.Status == "ready_to_start"
	isFullyPaidLead := detail.Lead.Status == "paid_full"
	hasZeroValueOffer := detail.Offer != nil && detail.Offer.FinalPrice.Valid && detail.Offer.FinalPrice.Int32 == 0
	return hasCredits || alreadyInWaitingFlow || isFullyPaidLead || hasZeroValueOffer
}

func canApproveZeroValueOffer(userRole string) bool {
	return userRole == "manager"
}

func canUseCoursePaymentFlow(detail *models.LeadDetail) (bool, string) {
	if detail == nil || detail.Lead == nil {
		return false, "Course payment is locked until lead details are loaded."
	}
	switch detail.Lead.Status {
	case "offer_sent", "booking_confirmed", "deposit_paid":
		return true, ""
	case "renewal_pending":
		return false, "Course payment is locked in renewal pending. Send packages first to move to Offer Sent."
	default:
		return false, "Course payment is locked at this stage. Move the lead to Offer Sent first."
	}
}

func isReturningCyclePlacementLocked(lead *models.Lead) bool {
	if lead == nil {
		return false
	}
	if lead.IsReturning {
		return true
	}
	switch lead.Status {
	case "renewal_pending", "waiting_for_round", "schedule_assigned", "ready_to_start", "in_classes":
		return true
	default:
		return false
	}
}

func isPaidWaitingFlowStatus(status string) bool {
	switch status {
	case "waiting_for_round", "schedule_assigned", "ready_to_start", "in_classes":
		return true
	default:
		return false
	}
}

func canMarkOfferSent(detail *models.LeadDetail) (bool, string) {
	if detail == nil || detail.Lead == nil {
		return false, "Lead data is unavailable. Please refresh and try again."
	}
	if isPaidWaitingFlowStatus(detail.Lead.Status) {
		return false, "Packages Sent is locked: this lead is already in paid waiting flow."
	}
	if computedRemainingCredits(detail.Lead) > 0 {
		return false, "Packages Sent is locked: this lead still has prepaid remaining credits."
	}

	// Check if placement test level is assigned by Student Success
	if detail.PlacementTest == nil || !detail.PlacementTest.AssignedLevel.Valid {
		return false, "Packages Sent is locked: Student Success must assign a placement test level first."
	}

	return true, ""
}

func canSetSleepingReminder(detail *models.LeadDetail) bool {
	if detail == nil || detail.Lead == nil || detail.Lead.Status != "lead_created" {
		return false
	}
	if detail.PlacementTest == nil {
		return true
	}
	return !detail.PlacementTest.TestDate.Valid && !detail.PlacementTest.TestTime.Valid
}

func canSnoozeLead(detail *models.LeadDetail) bool {
	if detail == nil || detail.Lead == nil {
		return false
	}
	if canSetSleepingReminder(detail) {
		return true
	}
	if detail.Lead.Status == "in_classes" || detail.Lead.Status == "cancelled" {
		return false
	}
	if detail.Lead.SentToClasses {
		return false
	}
	if detail.Lead.OpsQueueReason.Valid && detail.Lead.OpsQueueReason.String == "private_track" {
		return false
	}
	return true
}

func canSetOfferReminder(detail *models.LeadDetail, paymentState string) bool {
	if detail == nil || detail.Lead == nil {
		return false
	}
	return detail.Lead.Status == "offer_sent" && paymentState == models.PaymentStateUnpaid
}

func (h *PreEnrolmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	userID, _ := uuid.Parse(middleware.GetUserID(r))
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	// Read action parameter
	action := r.FormValue("action")
	h.cfg.Debugf("🔄 Update: leadID=%s, action=%s, userRole=%s, path=%s", leadID, action, userRole, r.URL.Path)

	// Handle different actions
	switch action {
	case "mark_test_booked":
		h.cfg.Debugf("  → Action: mark_test_booked")
		existingDetail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if isReturningCyclePlacementLocked(existingDetail.Lead) {
			h.renderDetailWithError(w, r, leadID, "Placement test booking is locked for returning students. Keep the promoted level and continue with renewal payment flow.")
			return
		}
		placementTest, err := buildBookedPlacementTestFromRequest(leadID, existingDetail.PlacementTest, r)
		if err != nil {
			h.renderDetailWithError(w, r, leadID, err.Error())
			return
		}

		err = models.BookPlacementTest(leadID, placementTest)
		if err != nil {
			log.Printf("ERROR: Failed to book placement test: %v", err)
			http.Error(w, "Couldn't book the placement test. Please try again.", http.StatusInternalServerError)
			return
		}
		if err := syncPlacementTestFinanceForBooking(leadID, placementTest); err != nil {
			log.Printf("ERROR: Failed to sync placement test finance transaction: %v", err)
			h.renderDetailWithError(w, r, leadID, "Couldn't sync the placement test payment. Please try again.")
			return
		}

		h.cfg.Debugf("  ✅ Test booked successfully, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=test_booked", http.StatusFound)
		return

	case "mark_tested":
		h.cfg.Debugf("  → Action: mark_tested")
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}
		if userRole == "admin" {
			h.renderDetailWithError(w, r, leadID, "Placement test results are recorded by Student Success. Please use the Student Success dashboard.")
			return
		}

		// Update placement test if fields are provided
		if r.FormValue("assigned_level") != "" || r.FormValue("test_notes") != "" {
			detail, err := models.GetLeadByID(leadID)
			if err != nil {
				http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
				return
			}

			if detail.PlacementTest == nil {
				detail.PlacementTest = &models.PlacementTest{LeadID: leadID}
			}

			if assignedLevel := r.FormValue("assigned_level"); assignedLevel != "" {
				level, err := strconv.Atoi(assignedLevel)
				if err != nil || !isValidAssignedLevel(level) {
					h.renderDetailWithError(w, r, leadID, "Invalid assigned level. Allowed: 1–10.")
					return
				}
				detail.PlacementTest.AssignedLevel = sql.NullInt32{Int32: int32(level), Valid: true}
			}
			if testNotes := r.FormValue("test_notes"); testNotes != "" {
				detail.PlacementTest.TestNotes = sql.NullString{String: testNotes, Valid: true}
			}

			if err := models.UpdatePlacementTest(detail.PlacementTest); err != nil {
				http.Error(w, "Couldn't update the placement test. Please try again.", http.StatusInternalServerError)
				return
			}
		}

		err = models.UpdateLeadStatus(leadID, "tested")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to tested, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=tested", http.StatusFound)
		return

	case "mark_landing_contacted":
		h.cfg.Debugf("  → Action: mark_landing_contacted")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}
		if userID == uuid.Nil {
			http.Error(w, "Couldn't identify the current user for this action.", http.StatusUnauthorized)
			return
		}
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if !isLandingLead(detail.Lead) {
			h.renderDetailWithError(w, r, leadID, "This action is available only for landing page leads.")
			return
		}
		if isLandingLeadContactedInCurrentStatus(detail.Lead) {
			http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID.String()), http.StatusFound)
			return
		}
		if err := models.MarkLandingPageLeadContacted(leadID, userID); err != nil {
			log.Printf("ERROR: Failed to mark landing lead contacted: %v", err)
			http.Error(w, "Couldn't update the landing lead contact status. Please try again.", http.StatusInternalServerError)
			return
		}
		if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
			LeadID:          leadID,
			Channel:         "manual",
			EventType:       "contact_confirmed",
			Source:          "landing_page_contacted",
			MessageText:     fmt.Sprintf("Lead marked as contacted while status was %s.", detail.Lead.Status),
			Metadata:        map[string]interface{}{"status": detail.Lead.Status},
			CreatedByUserID: &userID,
		}); err != nil {
			log.Printf("ERROR: Failed to log landing lead contacted event for lead %s: %v", leadID, err)
		}
		http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?landing_contacted=1", leadID.String()), http.StatusFound)
		return

	case "mark_offer_sent":
		h.cfg.Debugf("  → Action: mark_offer_sent")
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}

		// Update or create offer
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if allowed, reason := canMarkOfferSent(detail); !allowed {
			h.renderDetailWithError(w, r, leadID, reason)
			return
		}
		coursePaymentEnabled, _ := canUseCoursePaymentFlow(detail)

		// Bundle is optional for Packages Sent. Only use payment pricing fields when the
		// course-payment panel is actually active; otherwise hidden payment fields from the
		// locked renewal screen would incorrectly force bundle/final-price validation.
		bundle := strings.TrimSpace(r.FormValue("bundle"))
		finalPrice := strings.TrimSpace(r.FormValue("final_price"))
		basePrice := strings.TrimSpace(r.FormValue("base_price"))
		discountValue := strings.TrimSpace(r.FormValue("discount"))
		discountType := strings.ToLower(strings.TrimSpace(r.FormValue("discount_type")))
		if coursePaymentEnabled {
			if bundle == "" {
				bundle = firstNonEmpty(r.FormValue("bundle_id"), r.FormValue("payment_bundle"))
			}
			if finalPrice == "" {
				finalPrice = strings.TrimSpace(r.FormValue("payment_final_price"))
			}
			if basePrice == "" {
				basePrice = strings.TrimSpace(r.FormValue("payment_base_price"))
			}
			if discountValue == "" {
				discountValue = firstNonEmpty(r.FormValue("discount_amount"), r.FormValue("payment_discount_amount"))
			}
			if discountType == "" {
				discountType = strings.ToLower(firstNonEmpty(r.FormValue("payment_discount_type"), r.FormValue("discount_type")))
			}
		}

		// If bundle is selected, final_price must be set (0 is valid, empty is not).
		if bundle != "" && finalPrice == "" {
			log.Printf("ERROR: Validation failed for mark_offer_sent: bundle selected but no final_price")
			h.renderDetailWithError(w, r, leadID, "Please set Final Price for the selected bundle.")
			return
		}

		// If final_price is set, bundle must be selected.
		if finalPrice != "" && bundle == "" {
			log.Printf("ERROR: Validation failed for mark_offer_sent: final_price set but no bundle")
			h.renderDetailWithError(w, r, leadID, "Please select a bundle for the specified price.")
			return
		}

		// Persist Booking & Materials in the same submit (important for "Packages Sent" flow).
		bookFormat := strings.ToLower(strings.TrimSpace(r.FormValue("book_format")))
		if bookFormat != "" {
			if bookFormat != "pdf" && bookFormat != "printed" {
				h.renderDetailWithError(w, r, leadID, "Invalid book format. Allowed values are PDF or Printed.")
				return
			}

			booking := &models.Booking{
				LeadID:     leadID,
				BookFormat: sql.NullString{String: bookFormat, Valid: true},
			}
			var shipping *models.Shipping

			if bookFormat == "pdf" {
				booking.Address = sql.NullString{Valid: false}
				booking.City = sql.NullString{Valid: false}
				booking.DeliveryNotes = sql.NullString{Valid: false}
				shipping = &models.Shipping{
					LeadID:         leadID,
					ShipmentStatus: sql.NullString{Valid: false},
					ShipmentDate:   sql.NullTime{Valid: false},
				}
			} else {
				if address := strings.TrimSpace(r.FormValue("address")); address != "" {
					booking.Address = sql.NullString{String: address, Valid: true}
				}
				if city := strings.TrimSpace(r.FormValue("city")); city != "" {
					booking.City = sql.NullString{String: city, Valid: true}
				}
				if notes := strings.TrimSpace(r.FormValue("delivery_notes")); notes != "" {
					booking.DeliveryNotes = sql.NullString{String: notes, Valid: true}
				}
			}

			if err := models.UpsertBookingAndShipping(booking, shipping); err != nil {
				http.Error(w, "Couldn't save booking/materials. Please try again.", http.StatusInternalServerError)
				return
			}
		}

		if detail.Offer == nil {
			detail.Offer = &models.Offer{LeadID: leadID}
		}
		if offerNotes := strings.TrimSpace(r.FormValue("offer_notes")); offerNotes != "" {
			detail.Offer.FollowUpNotes = sql.NullString{String: offerNotes, Valid: true}
		} else {
			detail.Offer.FollowUpNotes = sql.NullString{Valid: false}
		}

		if b, err := strconv.Atoi(bundle); err == nil {
			detail.Offer.BundleLevels = sql.NullInt32{Int32: int32(b), Valid: true}
		}
		if fp, err := strconv.Atoi(finalPrice); err == nil {
			detail.Offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
		}
		if basePrice != "" {
			if bp, err := strconv.Atoi(basePrice); err == nil {
				detail.Offer.BasePrice = sql.NullInt32{Int32: int32(bp), Valid: true}
			}
		}
		if discountValue != "" {
			if discountType == "percent" || strings.HasSuffix(discountValue, "%") {
				discountValue = strings.TrimSuffix(discountValue, "%")
				if pct, err := strconv.Atoi(discountValue); err == nil {
					detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
					detail.Offer.DiscountType = sql.NullString{String: "percent", Valid: true}
				}
			} else {
				if amt, err := strconv.Atoi(discountValue); err == nil {
					detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(amt), Valid: true}
					detail.Offer.DiscountType = sql.NullString{String: "amount", Valid: true}
				}
			}
		}
		if detail.Offer.FinalPrice.Valid && detail.Offer.FinalPrice.Int32 == 0 && !canApproveZeroValueOffer(userRole) {
			h.renderDetailWithError(w, r, leadID, "Only Manager can approve a zero-value offer.")
			return
		}

		if err := models.UpdateOffer(detail.Offer); err != nil {
			http.Error(w, "Couldn't update the offer. Please try again.", http.StatusInternalServerError)
			return
		}

		err = models.UpdateLeadStatus(leadID, "offer_sent")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to offer_sent, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=offer_sent", http.StatusFound)
		return

	case "mark_cold_lead":
		h.cfg.Debugf("  → Action: mark_cold_lead")
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if detail.Lead.Status != "offer_sent" {
			h.renderDetailWithError(w, r, leadID, "Cold leads can only be set for offers that haven't been accepted.")
			return
		}
		coldAnchor := detail.Lead.UpdatedAt
		if detail.Lead.OfferSentAt.Valid {
			coldAnchor = detail.Lead.OfferSentAt.Time
		}
		if time.Since(coldAnchor) < 7*24*time.Hour {
			h.renderDetailWithError(w, r, leadID, "Cold leads can only be sent after 7 days with no response.")
			return
		}

		if err := models.UpdateLeadStatus(leadID, "cold_lead"); err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to cold_lead, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?cold=1&status_flash=cold", http.StatusFound)
		return

	case "mark_refused_renewal":
		h.cfg.Debugf("  → Action: mark_refused_renewal")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if !detail.Lead.IsReturning {
			h.renderDetailWithError(w, r, leadID, "Refused renewal action is only available for returning students.")
			return
		}
		if computedRemainingCredits(detail.Lead) > 0 {
			h.renderDetailWithError(w, r, leadID, "This student still has prepaid credits. Use waiting flow, not refused renewal.")
			return
		}
		if detail.Lead.Status != "renewal_pending" && detail.Lead.Status != "offer_sent" {
			h.renderDetailWithError(w, r, leadID, "Refused renewal action is only allowed during renewal flow (renewal pending / offer sent).")
			return
		}

		var actorID *uuid.UUID
		if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
			if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
				actorID = &parsed
			}
		}
		reason := strings.TrimSpace(r.FormValue("refused_renewal_reason"))
		otherReasonText := strings.TrimSpace(r.FormValue("refused_renewal_other_reason"))
		if !models.IsValidRefusedRenewalReason(reason) {
			h.renderDetailWithError(w, r, leadID, "Please choose a refusal reason.")
			return
		}
		if reason == models.RefusedRenewalReasonOther && otherReasonText == "" {
			h.renderDetailWithError(w, r, leadID, "Please write the reason when choosing Other.")
			return
		}
		if err := models.MarkRenewalRefusedAndSetCold(leadID, actorID, reason, otherReasonText); err != nil {
			log.Printf("ERROR: Failed to mark renewal refused: %v", err)
			http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Lead marked refused_renewal and moved to cold_lead")
		http.Redirect(w, r, "/pre-enrolment?cold=1&status_flash=refused", http.StatusFound)
		return

	case "send_refused_renewal_message":
		h.cfg.Debugf("  → Action: send_refused_renewal_message")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to send refused renewal messages.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if detail == nil || detail.Lead == nil || detail.Lead.Status != "cold_lead" {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "This lead is not in cold leads anymore.")
			return
		}

		latestRefusal, err := models.GetLatestRenewalRefusal(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to load renewal refusal for lead %s: %v", leadID, err)
			redirectWithError(w, r, "/pre-enrolment?cold=1", "Couldn't load refused renewal state.")
			return
		}
		if latestRefusal == nil {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "This lead does not have a refused renewal record.")
			return
		}

		lastStep, lastSentAt, err := models.GetLatestRefusedRenewalFollowUp(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to load refused renewal follow-up state for lead %s: %v", leadID, err)
			redirectWithError(w, r, "/pre-enrolment?cold=1", "Couldn't load refused renewal follow-up state.")
			return
		}
		nextStep, _, dueNow, _ := models.ComputeRefusedRenewalFollowUpState(latestRefusal.RefusedAt, lastStep, lastSentAt, util.CairoNow())
		if !dueNow {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "This refused renewal follow-up is not due yet.")
			return
		}

		requestedStep, err := strconv.Atoi(strings.TrimSpace(r.FormValue("refused_follow_up_step")))
		if err != nil || requestedStep < 1 || requestedStep > 3 {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "Invalid refused renewal message step.")
			return
		}
		if nextStep != requestedStep && !(nextStep == 3 && requestedStep == 3) {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "This refused renewal message step is no longer available.")
			return
		}

		messageText := strings.TrimSpace(firstNonEmpty(r.FormValue("refused_follow_up_message"), r.FormValue("message_bank_text")))
		if messageText == "" {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "Please prepare the WhatsApp message before sending.")
			return
		}

		whatsAppURL := buildWhatsAppComposeLink(detail.Lead.Phone, messageText)
		if whatsAppURL == "" {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "This lead does not have a valid WhatsApp number.")
			return
		}

		userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(r)))
		if err != nil {
			redirectWithError(w, r, "/pre-enrolment?cold=1", "Couldn't identify the current user for follow-up logging.")
			return
		}

		var templateID *uuid.UUID
		templateKey := ""
		templateIDRaw := strings.TrimSpace(r.FormValue("refused_follow_up_template_id"))
		if templateIDRaw != "" {
			parsedTemplateID, parseErr := uuid.Parse(templateIDRaw)
			if parseErr != nil {
				redirectWithError(w, r, "/pre-enrolment?cold=1", "Invalid message template.")
				return
			}
			template, templateErr := models.GetRefusedRenewalMessageTemplateByID(parsedTemplateID)
			if templateErr != nil {
				log.Printf("ERROR: Failed to load refused renewal message template %s: %v", parsedTemplateID, templateErr)
				redirectWithError(w, r, "/pre-enrolment?cold=1", "Couldn't load the selected message template.")
				return
			}
			if template == nil {
				redirectWithError(w, r, "/pre-enrolment?cold=1", "Selected message template was not found.")
				return
			}
			templateID = &parsedTemplateID
			templateKey = template.TemplateKey
		}

		if err := models.RecordRefusedRenewalFollowUp(leadID, requestedStep, templateID, messageText, userID); err != nil {
			log.Printf("ERROR: Failed to record refused renewal follow-up for lead %s: %v", leadID, err)
			redirectWithError(w, r, "/pre-enrolment?cold=1", "Couldn't record this refused renewal follow-up.")
			return
		}
		if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
			LeadID:      leadID,
			Channel:     "whatsapp",
			EventType:   "message_ready",
			Source:      "refused_renewal_sequence",
			TemplateKey: templateKey,
			MessageText: messageText,
			Metadata: map[string]interface{}{
				"step":           requestedStep,
				"refusal_reason": latestRefusal.Reason,
			},
			CreatedByUserID: &userID,
		}); err != nil {
			log.Printf("ERROR: Failed to log refused renewal contact history for lead %s: %v", leadID, err)
		}

		http.Redirect(w, r, whatsAppURL, http.StatusFound)
		return

	case "send_renewal_pending_message":
		h.cfg.Debugf("  → Action: send_renewal_pending_message")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to send renewal pending messages.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if detail == nil || detail.Lead == nil || detail.Lead.Status != "renewal_pending" || !detail.Lead.IsReturning {
			h.renderDetailWithError(w, r, leadID, "رسالة التجديد دي متاحة بس للطلبة العائدين اللي لسه في Renewal Pending.")
			return
		}

		levelValue, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("renewal_pending_level")))
		if levelValue <= 0 {
			h.renderDetailWithError(w, r, leadID, "مافيش مستوى مكتمل نقدر نبني عليه رسالة التجديد.")
			return
		}

		templateKey := strings.TrimSpace(r.FormValue("renewal_pending_template_key"))
		templateLabel := renewalPendingLabelForKey(templateKey)
		if templateLabel == "" {
			h.renderDetailWithError(w, r, leadID, "مافيش رسالة تجديد مناسبة للحالة دي.")
			return
		}

		messageText := strings.TrimSpace(firstNonEmpty(r.FormValue("renewal_pending_message_text"), r.FormValue("message_bank_text")))
		if messageText == "" {
			h.renderDetailWithError(w, r, leadID, "من فضلك جهز رسالة التجديد قبل الإرسال.")
			return
		}

		whatsAppURL := buildWhatsAppComposeLink(detail.Lead.Phone, messageText)
		if whatsAppURL == "" {
			h.renderDetailWithError(w, r, leadID, "This lead does not have a valid WhatsApp number.")
			return
		}

		userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(r)))
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Couldn't identify the current user for follow-up logging.")
			return
		}

		if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
			LeadID:      leadID,
			Channel:     "whatsapp",
			EventType:   "message_ready",
			Source:      "renewal_pending_post_level",
			TemplateKey: templateKey,
			MessageText: messageText,
			Metadata: map[string]interface{}{
				"message_label":   templateLabel,
				"completed_level": levelValue,
			},
			CreatedByUserID: &userID,
		}); err != nil {
			log.Printf("ERROR: Failed to log renewal pending contact history for lead %s: %v", leadID, err)
		}

		http.Redirect(w, r, whatsAppURL, http.StatusFound)
		return

	case "send_sleeping_lead_message":
		h.cfg.Debugf("  → Action: send_sleeping_lead_message")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to send sleeping lead messages.", http.StatusForbidden)
			return
		}

		item, err := models.GetSleepingLeadByID(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to load sleeping lead %s: %v", leadID, err)
			redirectWithError(w, r, "/pre-enrolment?sleeping=1", "Couldn't load this sleeping lead right now.")
			return
		}
		if item == nil || item.SleepingLeadStep < 1 || item.SleepingLeadStep > 3 || !item.SleepingLeadDueNow {
			h.renderDetailWithError(w, r, leadID, "رسالة Sleeping Leads الحالية مش مستحقة دلوقتي.")
			return
		}

		requestedStep, err := strconv.Atoi(strings.TrimSpace(r.FormValue("sleeping_follow_up_step")))
		if err != nil || requestedStep != item.SleepingLeadStep {
			h.renderDetailWithError(w, r, leadID, "مسموح تبعت بس رسالة الـ Sleeping Leads المستحقة حالياً.")
			return
		}

		messageText := strings.TrimSpace(r.FormValue("message_bank_text"))
		if messageText == "" {
			messageText = buildSleepingLeadMessage(item.Lead.FullName, item.SleepingLeadStep)
		}

		whatsAppURL := buildWhatsAppComposeLink(item.Lead.Phone, messageText)
		if whatsAppURL == "" {
			h.renderDetailWithError(w, r, leadID, "This lead does not have a valid WhatsApp number.")
			return
		}

		userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(r)))
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Couldn't identify the current user for follow-up logging.")
			return
		}
		if err := models.RecordSleepingLeadFollowUp(leadID, item.SleepingLeadStep, userID); err != nil {
			log.Printf("ERROR: Failed to record sleeping follow-up for lead %s: %v", leadID, err)
			h.renderDetailWithError(w, r, leadID, "Couldn't record this sleeping lead follow-up.")
			return
		}
		if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
			LeadID:          leadID,
			Channel:         "whatsapp",
			EventType:       "message_ready",
			Source:          "sleeping_lead_sequence",
			TemplateKey:     fmt.Sprintf("sleeping_message_%d", item.SleepingLeadStep),
			MessageText:     messageText,
			Metadata:        map[string]interface{}{"step": item.SleepingLeadStep},
			CreatedByUserID: &userID,
		}); err != nil {
			log.Printf("ERROR: Failed to log sleeping lead contact history for lead %s: %v", leadID, err)
		}

		http.Redirect(w, r, whatsAppURL, http.StatusFound)
		return

	case "send_offer_sent_message":
		h.cfg.Debugf("  → Action: send_offer_sent_message")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to send offer follow-up messages.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if detail == nil || detail.Lead == nil || detail.Lead.Status != "offer_sent" {
			h.renderDetailWithError(w, r, leadID, "رسائل After Placement Test متاحة بس للحالات اللي في Offer Sent.")
			return
		}

		var amountPaid, finalPrice sql.NullInt32
		if detail.Payment != nil {
			amountPaid = detail.Payment.AmountPaid
		}
		if detail.Offer != nil {
			finalPrice = detail.Offer.FinalPrice
		}
		item := &models.LeadListItem{
			Lead:       detail.Lead,
			AmountPaid: amountPaid,
			FinalPrice: finalPrice,
		}
		lastStep, lastSentAt, followUpErr := models.GetLatestOfferSentFollowUp(leadID)
		if followUpErr != nil {
			log.Printf("ERROR: Failed to load latest offer follow-up for lead %s: %v", leadID, followUpErr)
			h.renderDetailWithError(w, r, leadID, "Couldn't load after placement test state.")
			return
		}
		item.OfferFollowUpLastStep = lastStep
		item.OfferFollowUpLastSent = lastSentAt
		offerReminder, reminderErr := models.GetOfferSentReminder(leadID)
		if reminderErr != nil {
			log.Printf("ERROR: Failed to load offer reminder for lead %s: %v", leadID, reminderErr)
			h.renderDetailWithError(w, r, leadID, "Couldn't load after placement test reminder state.")
			return
		}
		if offerReminder != nil {
			item.OfferReminderAt = sql.NullTime{Time: offerReminder.FollowUpAt, Valid: true}
			item.OfferReminderNote = offerReminder.Note
		}
		models.ComputeLeadFlags(item)
		if item.PaymentState != models.PaymentStateUnpaid || item.OfferFollowUpStep < 1 || item.OfferFollowUpStep > 3 || !item.OfferFollowUpDueNow {
			h.renderDetailWithError(w, r, leadID, "رسالة After Placement Test الحالية مش مستحقة دلوقتي.")
			return
		}

		requestedStep, err := strconv.Atoi(strings.TrimSpace(r.FormValue("offer_follow_up_step")))
		if err != nil || requestedStep != item.OfferFollowUpStep {
			h.renderDetailWithError(w, r, leadID, "مسموح تبعت بس رسالة After Placement Test المستحقة حالياً.")
			return
		}

		messageText := strings.TrimSpace(r.FormValue("message_bank_text"))
		if messageText == "" {
			messageText = buildOfferSentFollowUpMessage(item.Lead.FullName, item.OfferFollowUpStep)
		}

		whatsAppURL := buildWhatsAppComposeLink(item.Lead.Phone, messageText)
		if whatsAppURL == "" {
			h.renderDetailWithError(w, r, leadID, "This lead does not have a valid WhatsApp number.")
			return
		}

		userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(r)))
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Couldn't identify the current user for follow-up logging.")
			return
		}
		if err := models.RecordOfferSentFollowUp(leadID, item.OfferFollowUpStep, userID); err != nil {
			log.Printf("ERROR: Failed to record offer follow-up for lead %s: %v", leadID, err)
			h.renderDetailWithError(w, r, leadID, "Couldn't record this after placement test follow-up.")
			return
		}
		if err := models.RecordPreEnrolmentContactHistory(models.ContactHistoryLogInput{
			LeadID:          leadID,
			Channel:         "whatsapp",
			EventType:       "message_ready",
			Source:          "offer_sent_sequence",
			TemplateKey:     fmt.Sprintf("offer_message_%d", item.OfferFollowUpStep),
			MessageText:     messageText,
			Metadata:        map[string]interface{}{"step": item.OfferFollowUpStep},
			CreatedByUserID: &userID,
		}); err != nil {
			log.Printf("ERROR: Failed to log offer follow-up contact history for lead %s: %v", leadID, err)
		}

		http.Redirect(w, r, whatsAppURL, http.StatusFound)
		return

	case "set_lead_snooze":
		h.cfg.Debugf("  → Action: set_lead_snooze")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to snooze leads.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if !canSnoozeLead(detail) {
			h.renderDetailWithError(w, r, leadID, "تأجيل المتابعة متاح بس للحالات اللي في الـ Main Feed أو Sleeping Leads.")
			return
		}

		snoozeDate := strings.TrimSpace(r.FormValue("lead_snooze_date"))
		if snoozeDate == "" {
			h.renderDetailWithError(w, r, leadID, "من فضلك اختار تاريخ التذكير.")
			return
		}
		snoozedUntil, err := util.ParseDateCairo(snoozeDate)
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "تاريخ التذكير غير صحيح.")
			return
		}
		if snoozedUntil.Before(util.CairoStartOfDay(util.CairoNow())) {
			h.renderDetailWithError(w, r, leadID, "مينفعش تاريخ التذكير يكون في الماضي.")
			return
		}

		var actorID *uuid.UUID
		if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
			if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
				actorID = &parsed
			}
		}

		if err := models.UpsertLeadSnooze(leadID, snoozedUntil, r.FormValue("lead_snooze_note"), actorID); err != nil {
			log.Printf("ERROR: Failed to save lead snooze: %v", err)
			http.Error(w, "ماقدرناش نحفظ تأجيل المتابعة. حاول تاني.", http.StatusInternalServerError)
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", fmt.Sprintf("تم تأجيل المتابعة لحد %s.", util.FormatDateCairo(snoozedUntil)))
		return

	case "clear_lead_snooze":
		h.cfg.Debugf("  → Action: clear_lead_snooze")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to clear snoozed leads.", http.StatusForbidden)
			return
		}
		if err := models.DeleteLeadSnooze(leadID); err != nil {
			log.Printf("ERROR: Failed to clear lead snooze: %v", err)
			http.Error(w, "ماقدرناش نلغي تأجيل المتابعة. حاول تاني.", http.StatusInternalServerError)
			return
		}
		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", "تم إلغاء تأجيل المتابعة.")
		return

	case "set_sleeping_reminder":
		h.cfg.Debugf("  → Action: set_sleeping_reminder")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to schedule sleeping lead reminders.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if !canSetSleepingReminder(detail) {
			h.renderDetailWithError(w, r, leadID, "Callback reminders are only available for leads still waiting to book their placement test.")
			return
		}

		reminderDate := strings.TrimSpace(r.FormValue("sleeping_reminder_date"))
		if reminderDate == "" {
			h.renderDetailWithError(w, r, leadID, "Please choose the reminder date.")
			return
		}
		followUpAt, err := util.ParseDateCairo(reminderDate)
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Invalid reminder date.")
			return
		}
		if followUpAt.Before(util.CairoStartOfDay(util.CairoNow())) {
			h.renderDetailWithError(w, r, leadID, "Reminder date cannot be in the past.")
			return
		}

		var actorID *uuid.UUID
		if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
			if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
				actorID = &parsed
			}
		}

		if err := models.UpsertSleepingLeadReminder(leadID, followUpAt, r.FormValue("sleeping_reminder_note"), actorID); err != nil {
			log.Printf("ERROR: Failed to save sleeping lead reminder: %v", err)
			http.Error(w, "Couldn't save the sleeping lead reminder. Please try again.", http.StatusInternalServerError)
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", fmt.Sprintf("Callback reminder saved for %s.", util.FormatDateCairo(followUpAt)))
		return

	case "clear_sleeping_reminder":
		h.cfg.Debugf("  → Action: clear_sleeping_reminder")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to clear sleeping lead reminders.", http.StatusForbidden)
			return
		}

		if err := models.DeleteSleepingLeadReminder(leadID); err != nil {
			log.Printf("ERROR: Failed to clear sleeping lead reminder: %v", err)
			http.Error(w, "Couldn't clear the sleeping lead reminder. Please try again.", http.StatusInternalServerError)
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", "Callback reminder cleared.")
		return

	case "set_offer_reminder":
		h.cfg.Debugf("  → Action: set_offer_reminder")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to schedule offer reminders.", http.StatusForbidden)
			return
		}

		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		var amountPaid, finalPrice sql.NullInt32
		if detail.Payment != nil {
			amountPaid = detail.Payment.AmountPaid
		}
		if detail.Offer != nil {
			finalPrice = detail.Offer.FinalPrice
		}
		paymentState := models.GetPaymentState(amountPaid, finalPrice)
		if !canSetOfferReminder(detail, paymentState) {
			h.renderDetailWithError(w, r, leadID, "Offer reminders are available only for unpaid leads in Offer Sent.")
			return
		}

		reminderDate := strings.TrimSpace(r.FormValue("offer_reminder_date"))
		if reminderDate == "" {
			h.renderDetailWithError(w, r, leadID, "Please choose the reminder date.")
			return
		}
		followUpAt, err := util.ParseDateCairo(reminderDate)
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Invalid reminder date.")
			return
		}
		if followUpAt.Before(util.CairoStartOfDay(util.CairoNow())) {
			h.renderDetailWithError(w, r, leadID, "Reminder date cannot be in the past.")
			return
		}

		var actorID *uuid.UUID
		if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
			if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
				actorID = &parsed
			}
		}

		if err := models.UpsertOfferSentReminder(leadID, followUpAt, r.FormValue("offer_reminder_note"), actorID); err != nil {
			log.Printf("ERROR: Failed to save offer reminder: %v", err)
			http.Error(w, "Couldn't save the offer reminder. Please try again.", http.StatusInternalServerError)
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", fmt.Sprintf("Offer reminder saved for %s.", util.FormatDateCairo(followUpAt)))
		return

	case "clear_offer_reminder":
		h.cfg.Debugf("  → Action: clear_offer_reminder")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to clear offer reminders.", http.StatusForbidden)
			return
		}

		if err := models.DeleteOfferSentReminder(leadID); err != nil {
			log.Printf("ERROR: Failed to clear offer reminder: %v", err)
			http.Error(w, "Couldn't clear the offer reminder. Please try again.", http.StatusInternalServerError)
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", "Offer reminder cleared.")
		return

	case "move_waiting":
		h.cfg.Debugf("  → Action: move_waiting")
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}

		// Waiting list is valid for:
		// - fully paid group-track leads, and
		// - returning/prepaid leads carrying credits into the next round.
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		if !canUseWaitingFlow(detail) {
			h.renderDetailWithError(w, r, leadID, "Cannot move to waiting list: this lead must be fully paid or prepaid for the next round first.")
			return
		}

		err = models.UpdateLeadStatus(leadID, "waiting_for_round")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to waiting_for_round, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=waiting", http.StatusFound)
		return

	case "add_bundle_credit":
		h.cfg.Debugf("  → Action: add_bundle_credit")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to add bundle credit.", http.StatusForbidden)
			return
		}

		addedLevelsStr := strings.TrimSpace(r.FormValue("bundle_credit_levels"))
		amountStr := strings.TrimSpace(r.FormValue("bundle_credit_amount"))
		paymentMethod := strings.TrimSpace(r.FormValue("bundle_credit_method"))
		paymentDateStr := strings.TrimSpace(r.FormValue("bundle_credit_date"))
		notes := strings.TrimSpace(r.FormValue("bundle_credit_notes"))

		addedLevels, err := strconv.Atoi(addedLevelsStr)
		if err != nil || addedLevels < 1 || addedLevels > 4 {
			h.renderDetailWithError(w, r, leadID, "Choose how many levels to add (1 to 4).")
			return
		}
		amount, err := strconv.Atoi(amountStr)
		if err != nil || amount <= 0 {
			h.renderDetailWithError(w, r, leadID, "Enter a valid bundle credit payment amount.")
			return
		}
		if paymentMethod == "" {
			h.renderDetailWithError(w, r, leadID, "Choose a payment method for the bundle credit.")
			return
		}
		if paymentDateStr == "" {
			h.renderDetailWithError(w, r, leadID, "Choose a payment date for the bundle credit.")
			return
		}
		paymentDate, err := util.ParseDateLocal(paymentDateStr)
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Invalid bundle credit payment date.")
			return
		}

		if _, err := models.AddWaitingListBundleCredit(leadID, int32(addedLevels), int32(amount), paymentMethod, paymentDate, notes); err != nil {
			h.renderDetailWithError(w, r, leadID, err.Error())
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", fmt.Sprintf("Added %d bundle credit level(s).", addedLevels))
		return

	case "send_private_track":
		h.cfg.Debugf("  → Action: send_private_track")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to move this lead to private track.", http.StatusForbidden)
			return
		}

		if err := models.SendLeadToPrivateTrack(leadID); err != nil {
			redirectWithError(w, r, "/pre-enrolment", err.Error())
			return
		}

		redirectWithFlash(w, r, "/pre-enrolment", "success", "Lead moved to Private Track.")
		return

	case "return_to_admin_feed":
		h.cfg.Debugf("  → Action: return_to_admin_feed")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to return this lead to admin feed.", http.StatusForbidden)
			return
		}

		if err := models.ReturnPrivateTrackLeadToAdminFeed(leadID); err != nil {
			redirectWithError(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), err.Error())
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", "Lead returned to the admin feed.")
		return

	case "mark_ready":
		h.cfg.Debugf("  → Action: mark_ready")
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
			return
		}

		// Get lead detail to validate prerequisites
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}

		// Check if fully paid
		var finalPriceValue int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			finalPriceValue = detail.Offer.FinalPrice.Int32
		}

		var totalCoursePaid int32
		if detail.Lead.IsReturning {
			totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
		} else {
			totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
		}
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			totalCoursePaid = 0
		}

		// For returning students:
		// - waiting_for_round: Already paid via previous bundle, no payment needed
		// - renewal_pending: Need to pay for new offer
		// The credit was already consumed during promotion, so remaining_credits will be 0
		currentStatus := detail.Lead.Status
		isWaitingForRound := currentStatus == "waiting_for_round"
		hasCredits := computedRemainingCredits(detail.Lead) > 0
		isFullyPaid := (detail.Offer != nil && detail.Offer.FinalPrice.Valid && totalCoursePaid >= finalPriceValue) || hasCredits

		log.Printf("💳 PAYMENT CHECK for lead %s: status=%s, finalPrice=%d, totalPaid=%d, remainingCredits=%d, hasCredits=%v, isWaitingForRound=%v, isFullyPaid=%v",
			leadID, currentStatus, finalPriceValue, totalCoursePaid,
			func() int32 {
				if detail.Lead.RemainingCredits.Valid {
					return detail.Lead.RemainingCredits.Int32
				}
				return 0
			}(), hasCredits, isWaitingForRound, isFullyPaid)

		if !isFullyPaid {
			h.renderDetailWithError(w, r, leadID, "Cannot mark READY_TO_START before full payment. Course must be fully paid first.")
			return
		}

		// Check assigned level exists
		if detail.PlacementTest == nil || !detail.PlacementTest.AssignedLevel.Valid {
			h.renderDetailWithError(w, r, leadID, "Cannot mark READY_TO_START: Assigned level must be set first.")
			return
		}

		// Schedule required: both Class Days and Class Time must be present
		classDaysMR := r.FormValue("class_days")
		classTimeMR := r.FormValue("class_time")
		if classDaysMR == "" || classTimeMR == "" {
			h.renderDetailWithError(w, r, leadID, "Cannot mark READY_TO_START: Both Class Days and Class Time are required.")
			return
		}
		allowedClassDaysMR := map[string]bool{"Sun/Wed": true, "Sat/Tues": true, "Mon/Thu": true}
		allowedClassTimesMR := map[string]bool{"07:30": true, "10:00": true}
		if !allowedClassDaysMR[classDaysMR] {
			h.renderDetailWithError(w, r, leadID, "Invalid class days. Allowed: Sun/Wed, Sat/Tues, Mon/Thu.")
			return
		}
		if !allowedClassTimesMR[classTimeMR] {
			h.renderDetailWithError(w, r, leadID, "Invalid class time. Allowed: 07:30, 10:00.")
			return
		}

		if err := models.UpsertSchedulingClassDaysTime(leadID, classDaysMR, classTimeMR); err != nil {
			log.Printf("ERROR: Failed to save schedule: %v", err)
			http.Error(w, "Couldn't save the schedule. Please try again.", http.StatusInternalServerError)
			return
		}
		err = models.UpdateLeadStatus(leadID, "ready_to_start")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to ready_to_start, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=ready", http.StatusFound)
		return

	case "send_to_classes":
		h.cfg.Debugf("  → Action: send_to_classes")
		// Server-side check: moderators cannot send to classes
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to send leads to classes.", http.StatusForbidden)
			return
		}

		// Verify lead is ready (has level, days, time)
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}

		// Check eligibility: waiting-for-round and ready-to-start leads can be sent once schedule is set.
		if detail.Lead.Status != "ready_to_start" && detail.Lead.Status != "waiting_for_round" {
			h.renderDetailWithError(w, r, leadID, "Lead must be READY_TO_START or WAITING_FOR_ROUND to send to classes.")
			return
		}
		if detail.PlacementTest == nil || !detail.PlacementTest.AssignedLevel.Valid {
			h.renderDetailWithError(w, r, leadID, "Lead must have an assigned level to send to classes.")
			return
		}
		if detail.Scheduling == nil || !detail.Scheduling.ClassDays.Valid || !detail.Scheduling.ClassTime.Valid {
			h.renderDetailWithError(w, r, leadID, "Lead must have class days and class time set to send to classes.")
			return
		}

		// Send to classes
		err = models.SendLeadToClasses(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to send lead to classes: %v", err)
			// Check if AJAX request
			if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				if _, writeErr := w.Write([]byte(`{"success": false, "error": "Failed to send lead to classes"}`)); writeErr != nil {
					log.Printf("ERROR: Failed to write send-to-classes error response: %v", writeErr)
				}
				return
			}
			http.Error(w, "Couldn't send this lead to classes. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Lead sent to classes, redirecting to list")
		// Check if AJAX request - return JSON instead of redirect
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, writeErr := w.Write([]byte(`{"success": true, "message": "Lead sent to classes board successfully"}`)); writeErr != nil {
				log.Printf("ERROR: Failed to write send-to-classes success response: %v", writeErr)
			}
			return
		}
		http.Redirect(w, r, "/pre-enrolment?sentToClasses=1", http.StatusFound)
		return

	case "apply_continuation_hold":
		h.cfg.Debugf("  → Action: apply_continuation_hold")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to place this student on hold.", http.StatusForbidden)
			return
		}

		reason := strings.TrimSpace(r.FormValue("continuation_hold_reason"))
		if err := models.ApplyContinuationHold(leadID, userID, reason); err != nil {
			h.renderDetailWithError(w, r, leadID, err.Error())
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", "Continuation hold applied. The reserved next level was restored.")
		return

	case "release_continuation_hold":
		h.cfg.Debugf("  → Action: release_continuation_hold")
		if userRole != "admin" && userRole != "manager" {
			http.Error(w, "You don't have permission to release this hold.", http.StatusForbidden)
			return
		}

		if err := models.ReleaseContinuationHold(leadID, userID); err != nil {
			h.renderDetailWithError(w, r, leadID, err.Error())
			return
		}

		redirectWithFlash(w, r, fmt.Sprintf("/pre-enrolment/%s", leadID), "success", "Continuation hold released. The next level was consumed again and the student is ready to start.")
		return

	case "cancel":
		h.cfg.Debugf("  → Action: cancel")
		// Server-side check: moderators cannot cancel
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to cancel leads.", http.StatusForbidden)
			return
		}

		// Get lead detail for cancel modal
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}
		isRefundReview := isRefundReviewLead(detail.Lead)
		cancelFlowAction := "cancel"
		if isRefundReview {
			cancelFlowAction = "refund_review"
		}
		cancelFlowPath := fmt.Sprintf("/pre-enrolment/%s?action=%s", leadID.String(), cancelFlowAction)

		// If this is a POST with refund data, process cancellation + refund
		if r.Method == http.MethodPost {
			if isRefundReview && r.FormValue("refund_review_flow") != "1" {
				redirectWithError(w, r, cancelFlowPath, "This lead must be cancelled through the Refund Review flow.")
				return
			}

			refundAmountStr := r.FormValue("refund_amount")
			refundMethod := r.FormValue("refund_method")
			refundDateStr := r.FormValue("refund_date")
			refundNotes := r.FormValue("refund_notes")

			// Check if student has remaining credits (covers IsReturning, renewal_pending, waiting_for_round)
			hasRemainingCredits := computedRemainingCredits(detail.Lead) > 0

			// Get course payments total (current cycle only for students with remaining credits)
			var totalCoursePaid int32
			if hasRemainingCredits || detail.Lead.IsReturning {
				totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
			} else {
				totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
			}
			if err != nil {
				h.renderDetailWithError(w, r, leadID, "Couldn't calculate course payments. Please try again.")
				return
			}

			// Calculate unused credits value for students with remaining credits
			var unusedCreditsValue int32
			if hasRemainingCredits {
				breakdown, refundErr := models.GetUnusedCreditsRefundBreakdown(leadID)
				if refundErr != nil {
					log.Printf("ERROR: Failed to calculate unused credits refund: %v", refundErr)
					h.renderDetailWithError(w, r, leadID, "System error: Cannot calculate unused credits refund safely. Please contact support.")
					return
				}
				unusedCreditsValue = breakdown.UnusedCreditsValue
			}

			// Total refundable amount uses unused-credits valuation when present (no double count).
			totalRefundableAmount := computeCancelRefundableAmount(totalCoursePaid, unusedCreditsValue)

			// CRITICAL SAFEGUARD: Detect leakage for returning students with credits
			// If a returning student has credits but refund calculation returned 0, this is a bug
			if detail.Lead.IsReturning && hasRemainingCredits && totalRefundableAmount == 0 {
				log.Printf("🚨 LEAKAGE ALERT: Returning student %s (%s) has %d credits but refund calculation returned 0. totalCoursePaid=%d, unusedCreditsValue=%d",
					leadID, detail.Lead.FullName, detail.Lead.RemainingCredits.Int32, totalCoursePaid, unusedCreditsValue)
				// This should not happen with the fixed CalculateUnusedCreditsRefund, but if it does,
				// we need to prevent the cancellation from proceeding without a refund
				h.renderDetailWithError(w, r, leadID, "System error: Cannot calculate refund for returning student with credits. Please contact support.")
				return
			}

			// If there are refundable amounts, refund details are required.
			// Some browsers/DOM edge cases can submit without refund_amount even when prefilled,
			// so we default to full refundable amount instead of failing with refund_required.
			if totalRefundableAmount > 0 {
				refundAmount := int(totalRefundableAmount)
				if strings.TrimSpace(refundAmountStr) != "" {
					refundAmount, err = strconv.Atoi(refundAmountStr)
					if err != nil || refundAmount <= 0 {
						http.Redirect(w, r, fmt.Sprintf("%s&error=invalid_amount", cancelFlowPath), http.StatusFound)
						return
					}
				}

				if int32(refundAmount) > totalRefundableAmount {
					http.Redirect(w, r, fmt.Sprintf("%s&error=amount_exceeds&max=%d", cancelFlowPath, totalRefundableAmount), http.StatusFound)
					return
				}

				// Validate payment method
				if refundMethod == "" {
					http.Redirect(w, r, fmt.Sprintf("%s&error=method_required", cancelFlowPath), http.StatusFound)
					return
				}

				allowedMethods := map[string]bool{
					"vodafone_cash": true,
					"bank_transfer": true,
					"paypal":        true,
					"other":         true,
				}
				if !allowedMethods[refundMethod] {
					http.Redirect(w, r, fmt.Sprintf("%s&error=invalid_method", cancelFlowPath), http.StatusFound)
					return
				}

				// Validate refund date
				if refundDateStr == "" {
					http.Redirect(w, r, fmt.Sprintf("%s&error=date_required", cancelFlowPath), http.StatusFound)
					return
				}

				refundDate, err := util.ParseDateLocal(refundDateStr)
				if err != nil {
					http.Redirect(w, r, fmt.Sprintf("%s&error=invalid_date", cancelFlowPath), http.StatusFound)
					return
				}

				// Validate refund date is not in the future
				if err := util.ValidateNotFutureDate(refundDate); err != nil {
					http.Redirect(w, r, fmt.Sprintf("%s&error=future_date", cancelFlowPath), http.StatusFound)
					return
				}

				refundNotesText := "Refund for cancelled lead"
				if refundNotes != "" {
					refundNotesText = refundNotesText + ". " + refundNotes
				}
				err = models.CreateCancelRefundIdempotent(leadID, int32(refundAmount), refundMethod, refundDate, refundNotesText)
				if err != nil {
					log.Printf("ERROR: Failed to create cancel refund: %v", err)
					if err.Error() == "payment date cannot be in the future" {
						http.Redirect(w, r, fmt.Sprintf("%s&error=future_date", cancelFlowPath), http.StatusFound)
						return
					}
					if strings.Contains(err.Error(), "cannot exceed total course paid") || strings.Contains(err.Error(), "cannot exceed refundable amount") {
						http.Redirect(w, r, fmt.Sprintf("%s&error=amount_exceeds&max=%d", cancelFlowPath, totalRefundableAmount), http.StatusFound)
						return
					}
					http.Redirect(w, r, fmt.Sprintf("%s&error=refund_failed", cancelFlowPath), http.StatusFound)
					return
				}
			}

			err = models.CancelLead(leadID)
			if err != nil {
				log.Printf("ERROR: Failed to cancel lead: %v", err)
				http.Error(w, "Couldn't cancel this lead. Please try again.", http.StatusInternalServerError)
				return
			}

			h.cfg.Debugf("  ✅ Lead cancelled successfully, redirecting to list")
			if totalCoursePaid > 0 {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?cancelled=1&refund_recorded=1", leadID.String()), http.StatusFound)
			} else {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?cancelled=1", leadID.String()), http.StatusFound)
			}
			return
		}

		// GET request: show cancel modal with refund options
		// Calculate placement test paid (read-only, not refundable)
		var placementTestPaid int32 = 0
		if detail.PlacementTest != nil && detail.PlacementTest.PlacementTestFeePaid.Valid {
			placementTestPaid = detail.PlacementTest.PlacementTestFeePaid.Int32
		}

		// Check if student has remaining credits (covers IsReturning, renewal_pending, waiting_for_round)
		hasRemainingCredits := computedRemainingCredits(detail.Lead) > 0

		// Calculate course paid total (current cycle for students with remaining credits)
		var totalCoursePaid int32
		if hasRemainingCredits || detail.Lead.IsReturning {
			totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
		} else {
			totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
		}
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			totalCoursePaid = 0
		}

		// Calculate unused credits value for students with remaining credits
		var unusedCreditsValue int32 = 0
		var calculatedRemainingCredits int32 = 0
		var consumedLevelsForRefund int32 = 0
		var consumedValueForRefund int32 = 0
		var originalPaidForRefund int32 = 0
		if hasRemainingCredits {
			// Calculate dynamic remaining credits for display
			if detail.Lead.LevelsPurchasedTotal.Valid && detail.Lead.LevelsConsumed.Valid {
				calculatedRemainingCredits = detail.Lead.LevelsPurchasedTotal.Int32 - detail.Lead.LevelsConsumed.Int32
				if calculatedRemainingCredits < 0 {
					calculatedRemainingCredits = 0
				}
			}
			breakdown, refundErr := models.GetUnusedCreditsRefundBreakdown(leadID)
			if refundErr != nil {
				log.Printf("ERROR: Failed to calculate unused credits refund: %v", refundErr)
				h.renderDetailWithError(w, r, leadID, "System error: Cannot calculate unused credits refund safely. Please contact support.")
				return
			}
			unusedCreditsValue = breakdown.UnusedCreditsValue
			if breakdown.RemainingCredits > 0 {
				calculatedRemainingCredits = breakdown.RemainingCredits
			}
			consumedLevelsForRefund = breakdown.ConsumedLevels
			consumedValueForRefund = breakdown.ConsumedValue
			originalPaidForRefund = breakdown.OriginalPaidValue
		}

		// Total refundable amount uses unused-credits valuation when present (no double count).
		totalRefundableAmount := computeCancelRefundableAmount(totalCoursePaid, unusedCreditsValue)

		// Get offer final price for remaining balance calculation
		// Get offer final price for remaining balance calculation
		// Use -1 to indicate "not applicable" when FinalPrice is not set
		// (0 means "paid in full", which is different from "price not set yet")
		var remainingBalance int32 = -1
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			remainingBalance = detail.Offer.FinalPrice.Int32 - totalCoursePaid
			if remainingBalance < 0 {
				remainingBalance = 0
			}
		}

		// Get lead payments for display
		leadPayments, err := models.GetLeadPayments(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get lead payments: %v", err)
			leadPayments = []*models.LeadPayment{} // Empty slice on error
		}

		// Calculate final price
		var finalPriceValue int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			finalPriceValue = detail.Offer.FinalPrice.Int32
		}

		today := time.Now().Format("2006-01-02")
		data := map[string]interface{}{
			"Title":                   fmt.Sprintf("Cancel Lead - %s", detail.Lead.FullName),
			"Detail":                  detail,
			"UserRole":                userRole,
			"IsModerator":             false,
			"ShowCancelModal":         true,
			"PlacementTestPaid":       placementTestPaid,
			"TotalCoursePaid":         totalCoursePaid,
			"UnusedCreditsValue":      unusedCreditsValue,
			"RemainingCreditsCount":   calculatedRemainingCredits,
			"ConsumedLevelsForRefund": consumedLevelsForRefund,
			"ConsumedValueForRefund":  consumedValueForRefund,
			"OriginalPaidForRefund":   originalPaidForRefund,
			"TotalRefundableAmount":   totalRefundableAmount,
			"RemainingBalance":        remainingBalance,
			"FinalPrice":              finalPriceValue,
			"LeadPayments":            leadPayments,
			"Today":                   today,
		}
		renderTemplate(w, r, "pre_enrolment_detail.html", data)
		return

	case "reopen":
		h.cfg.Debugf("  → Action: reopen")
		// Server-side check: moderators cannot reopen
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to reopen leads.", http.StatusForbidden)
			return
		}

		// Reopen the cancelled lead
		err = models.ReopenLead(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to reopen lead: %v", err)
			http.Error(w, "Couldn't reopen this lead. Please try again.", http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Lead reopened successfully, redirecting to detail")
		http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?reopened=1", leadID.String()), http.StatusFound)
		return

	case "delete":
		h.cfg.Debugf("  → Action: delete")
		if userRole == "moderator" {
			http.Error(w, "You don't have permission to delete leads.", http.StatusForbidden)
			return
		}
		// Direct delete is disabled; route to cancel flow so refund modal is enforced.
		http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel", leadID.String()), http.StatusFound)
		return

	case "save", "":
		// Default action: save all fields without forcing status change
		h.cfg.Debugf("  → Action: save (default)")
		// Use SaveFull logic but allow moderators for basic info only
		h.SaveFull(w, r)
		return

	default:
		h.cfg.Debugf("  ⚠️  Unknown action: %s, treating as save", action)
		// Unknown action, treat as save
		h.SaveFull(w, r)
		return
	}
}

func (h *PreEnrolmentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	status := r.FormValue("status")
	if status == "" {
		http.Error(w, "Please select a status.", http.StatusBadRequest)
		return
	}

	err = models.UpdateLeadStatus(leadID, status)
	if err != nil {
		http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?saved=1", http.StatusFound)
}

// SaveFull performs a full save of all form fields and redirects to list.
// IMPORTANT: This function now auto-classifies stage based on form completion.
// Stage is computed from the furthest completed block and automatically upgraded.
// Never downgrades stage - only upgrades based on what's filled.
// Validation: only validates basic lead fields (name, phone) + final_price if stage reaches OFFER_SENT
// Does NOT require offer/pricing fields for test booking - can save test info without offer
func (h *PreEnrolmentHandler) SaveFull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	h.cfg.Debugf("💾 SaveFull: leadID=%s, userRole=%s", leadID, userRole)
	returnToListAfterSave := r.FormValue("return_to_list") == "1"

	// Validate basic lead fields (name and phone are required)
	// Load existing lead first to get current values if form fields are missing
	existingDetail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
		return
	}

	fullName := r.FormValue("full_name")
	phone := r.FormValue("phone")

	// If fields are empty, use existing values (might happen with some form submissions)
	if fullName == "" {
		fullName = existingDetail.Lead.FullName
		h.cfg.Debugf("  ⚠️  full_name empty in form, using existing: %q", fullName)
	}
	if phone == "" {
		phone = existingDetail.Lead.Phone
		h.cfg.Debugf("  ⚠️  phone empty in form, using existing: %q", phone)
	}

	if fullName == "" || phone == "" {
		log.Printf("ERROR: Validation failed for SaveFull: fullName=%q, phone=%q, leadID=%s", fullName, phone, leadID)
		http.Error(w, "Full name and phone are required.", http.StatusBadRequest)
		return
	}

	// Parse form values
	detail := &models.LeadDetail{
		Lead: &models.Lead{
			ID:                   leadID,
			FullName:             fullName,
			Phone:                phone,
			Status:               existingDetail.Lead.Status,
			SentToClasses:        existingDetail.Lead.SentToClasses,
			IsReturning:          existingDetail.Lead.IsReturning,
			LevelsPurchasedTotal: existingDetail.Lead.LevelsPurchasedTotal,
			LevelsConsumed:       existingDetail.Lead.LevelsConsumed,
			RemainingCredits:     existingDetail.Lead.RemainingCredits,
		},
	}

	// Moderator restrictions: only allow editing Lead Info (name, phone, source, notes)
	if userRole == "moderator" {
		h.cfg.Debugf("  🔒 Moderator save: only updating Lead Info section")
		source := r.FormValue("source")
		allowedSources := map[string]bool{
			"Facebook": true, "WhatsApp": true, "Instagram": true, "Referral": true, "Walk-in": true, "Other": true,
		}
		if source != "" && allowedSources[source] {
			detail.Lead.Source = sql.NullString{String: source, Valid: true}
		} else if source != "" {
			detail.Lead.Source = sql.NullString{String: "Other", Valid: true}
		} else {
			detail.Lead.Source = sql.NullString{Valid: false}
		}
		notes := r.FormValue("notes")
		if notes != "" {
			detail.Lead.Notes = sql.NullString{String: notes, Valid: true}
		} else {
			detail.Lead.Notes = sql.NullString{Valid: false}
		}

		err = models.UpdateLeadBasicInfo(detail.Lead)
		if err != nil {
			if models.IsPhoneConstraintError(err) != nil {
				redir := fmt.Sprintf("/pre-enrolment/%s?error=phone_exists", leadID.String())
				if existingLead, findErr := models.GetLeadByPhone(phone); findErr == nil {
					redir = fmt.Sprintf("%s&existing_lead_id=%s", redir, existingLead.ID.String())
				}
				http.Redirect(w, r, redir, http.StatusFound)
				return
			}
			log.Printf("ERROR: Failed to update lead (moderator): %v", err)
			http.Error(w, "Couldn't update this lead. Please try again.", http.StatusInternalServerError)
			return
		}
		h.cfg.Debugf("  ✅ Moderator save successful")
		if returnToListAfterSave || existingDetail.Lead.Status == "lead_created" {
			http.Redirect(w, r, "/pre-enrolment?saved=1", http.StatusFound)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?saved=1", leadID.String()), http.StatusFound)
		return
	}

	// Admin: can update all sections
	h.cfg.Debugf("  👤 Admin save: updating all sections")

	if source := r.FormValue("source"); source != "" {
		// Validate source is one of allowed options
		allowedSources := map[string]bool{
			"Facebook":  true,
			"WhatsApp":  true,
			"Instagram": true,
			"Referral":  true,
			"Walk-in":   true,
			"Other":     true,
		}
		if !allowedSources[source] {
			source = "Other" // Default to Other if invalid
		}
		detail.Lead.Source = sql.NullString{String: source, Valid: true}
	}
	if notes := r.FormValue("notes"); notes != "" {
		detail.Lead.Notes = sql.NullString{String: notes, Valid: true}
	}

	// existingDetail already loaded above for validation
	currentStatus := existingDetail.Lead.Status

	// Auto-compute stage from form completion (before parsing all sections)
	// This will be used after all sections are parsed

	// Placement test
	if r.FormValue("test_date") != "" ||
		r.FormValue("assigned_level") != "" ||
		r.FormValue("placement_test_fee") != "" ||
		r.FormValue("placement_test_fee_paid") != "" ||
		r.FormValue("placement_test_payment_date") != "" ||
		r.FormValue("placement_test_payment_method") != "" ||
		r.FormValue("placement_test_discount_value") != "" ||
		r.FormValue("placement_test_discount_type") != "" {
		pt := &models.PlacementTest{LeadID: leadID}

		testDate := r.FormValue("test_date")
		testTime := r.FormValue("test_time")

		if testDate != "" {
			if t, err := time.Parse("2006-01-02", testDate); err == nil {
				pt.TestDate = sql.NullTime{Time: t, Valid: true}
			}
		}
		if testTime != "" {
			pt.TestTime = sql.NullString{String: testTime, Valid: true}
		}
		if testType := r.FormValue("test_type"); testType != "" {
			pt.TestType = sql.NullString{String: testType, Valid: true}
		}
		if assignedLevel := r.FormValue("assigned_level"); assignedLevel != "" {
			level, err := strconv.Atoi(assignedLevel)
			if err != nil || !isValidAssignedLevel(level) {
				h.renderDetailWithError(w, r, leadID, "Invalid assigned level. Allowed: 1–10.")
				return
			}

			// Non-Student-Success users may submit the already-assigned level as part of the
			// shared save form. Preserve the existing value, but reject attempts to change it.
			if userRole != "student_success" {
				existingSameLevel := existingDetail.PlacementTest != nil &&
					existingDetail.PlacementTest.AssignedLevel.Valid &&
					int(existingDetail.PlacementTest.AssignedLevel.Int32) == level
				if !existingSameLevel {
					h.renderDetailWithError(w, r, leadID, "Only Student Success can assign a level after conducting the placement test.")
					return
				}
			}

			pt.AssignedLevel = sql.NullInt32{Int32: int32(level), Valid: true}
		}
		if testNotes := r.FormValue("test_notes"); testNotes != "" {
			// Non-Student-Success users may submit existing notes from the shared save form,
			// but they cannot change them.
			if userRole != "student_success" {
				existingSameNotes := existingDetail.PlacementTest != nil &&
					existingDetail.PlacementTest.TestNotes.Valid &&
					existingDetail.PlacementTest.TestNotes.String == testNotes
				if !existingSameNotes {
					h.renderDetailWithError(w, r, leadID, "Only Student Success can add test notes after conducting the placement test.")
					return
				}
			}
			pt.TestNotes = sql.NullString{String: testNotes, Valid: true}
		}
		// Preserve existing assigned level/notes when fields are not submitted (admin fields disabled)
		if existingDetail.PlacementTest != nil {
			if !pt.AssignedLevel.Valid && existingDetail.PlacementTest.AssignedLevel.Valid {
				pt.AssignedLevel = existingDetail.PlacementTest.AssignedLevel
			}
			if !pt.TestNotes.Valid && existingDetail.PlacementTest.TestNotes.Valid {
				pt.TestNotes = existingDetail.PlacementTest.TestNotes
			}
		}
		// Placement test fee fields
		if feeStr := r.FormValue("placement_test_fee"); feeStr != "" {
			if fee, err := strconv.Atoi(feeStr); err == nil {
				pt.PlacementTestFee = sql.NullInt32{Int32: int32(fee), Valid: true}
			}
		}
		if paidStr := r.FormValue("placement_test_fee_paid"); paidStr != "" {
			if paid, err := strconv.Atoi(paidStr); err == nil {
				pt.PlacementTestFeePaid = sql.NullInt32{Int32: int32(paid), Valid: true}
			}
		}

		// Discount fields
		if discountValue := r.FormValue("placement_test_discount_value"); discountValue != "" {
			if dv, err := strconv.Atoi(discountValue); err == nil {
				pt.DiscountValue = sql.NullInt32{Int32: int32(dv), Valid: true}
			}
		}
		if discountType := r.FormValue("placement_test_discount_type"); discountType != "" {
			if discountType == "amount" || discountType == "percent" {
				pt.DiscountType = sql.NullString{String: discountType, Valid: true}
			}
		}
		if existingDetail.PlacementTest != nil {
			if !pt.DiscountValue.Valid && existingDetail.PlacementTest.DiscountValue.Valid {
				pt.DiscountValue = existingDetail.PlacementTest.DiscountValue
			}
			if !pt.DiscountType.Valid && existingDetail.PlacementTest.DiscountType.Valid {
				pt.DiscountType = existingDetail.PlacementTest.DiscountType
			}
		}

		// Normalize placement test fee/paid: paid must be 0 or equal to discounted final fee.
		feeValue := int32(60)
		if pt.PlacementTestFee.Valid {
			feeValue = pt.PlacementTestFee.Int32
		}
		finalPlacementFee := computePlacementTestFinalFee(feeValue, pt.DiscountValue, pt.DiscountType)
		if pt.TestDate.Valid && pt.TestTime.Valid && finalPlacementFee > 0 {
			if !pt.PlacementTestFeePaid.Valid || pt.PlacementTestFeePaid.Int32 <= 0 {
				h.renderDetailWithError(w, r, leadID, "Paid amount is required before saving the placement test as booked.")
				return
			}
			if pt.PlacementTestFeePaid.Int32 != finalPlacementFee {
				h.renderDetailWithError(w, r, leadID, fmt.Sprintf("Paid amount must equal the final placement test fee (%d EGP) before booking the test.", finalPlacementFee))
				return
			}
		}
		if pt.PlacementTestFeePaid.Valid && pt.PlacementTestFeePaid.Int32 > 0 {
			if !pt.PlacementTestFee.Valid {
				pt.PlacementTestFee = sql.NullInt32{Int32: feeValue, Valid: true}
			}
		}

		// Payment method/date: required only if paid > 0. Otherwise keep them NULL.
		amountPaid := int32(0)
		if pt.PlacementTestFeePaid.Valid {
			amountPaid = pt.PlacementTestFeePaid.Int32
		}
		if amountPaid > 0 {
			paymentDateStr := r.FormValue("placement_test_payment_date")
			if paymentDateStr == "" {
				h.renderDetailWithError(w, r, leadID, "Payment date is required when placement test fee is paid.")
				return
			}
			t, err := util.ParseDateLocal(paymentDateStr)
			if err != nil {
				h.renderDetailWithError(w, r, leadID, "Invalid payment date for placement test.")
				return
			}
			if err := util.ValidateNotFutureDate(t); err != nil {
				h.renderDetailWithError(w, r, leadID, "Payment date cannot be in the future")
				return
			}
			pt.PlacementTestPaymentDate = sql.NullTime{Time: t, Valid: true}

			paymentMethod := r.FormValue("placement_test_payment_method")
			if paymentMethod == "" {
				h.renderDetailWithError(w, r, leadID, "Payment method is required when placement test fee is paid.")
				return
			}
			pt.PlacementTestPaymentMethod = sql.NullString{String: paymentMethod, Valid: true}
		} else {
			pt.PlacementTestPaymentDate = sql.NullTime{Valid: false}
			pt.PlacementTestPaymentMethod = sql.NullString{Valid: false}
		}

		detail.PlacementTest = pt
	}

	// Offer
	// Only process offer if bundle is explicitly provided OR if save_offer action is triggered
	// This prevents auto-selecting bundle when saving other sections (e.g., placement test payment)
	finalPriceStr := r.FormValue("final_price")
	bundleStr := r.FormValue("bundle")
	basePriceStr := r.FormValue("base_price")
	discountStr := r.FormValue("discount")
	paymentBundleStr := firstNonEmpty(r.FormValue("bundle_id"), r.FormValue("payment_bundle"))
	paymentDiscountAmountStr := firstNonEmpty(r.FormValue("discount_amount"), r.FormValue("payment_discount_amount"))
	paymentDiscountType := strings.ToLower(firstNonEmpty(r.FormValue("discount_type"), r.FormValue("payment_discount_type")))
	paymentFinalPriceStr := r.FormValue("payment_final_price")
	pricingTrack := normalizePricingTrack(firstNonEmpty(r.FormValue("pricing_track"), inferOfferPricingTrack(existingDetail.Offer)))
	coursePaymentEnabledForExisting, _ := canUseCoursePaymentFlow(existingDetail)
	if !coursePaymentEnabledForExisting {
		paymentBundleStr = ""
		paymentDiscountAmountStr = ""
		paymentDiscountType = ""
		paymentFinalPriceStr = ""
	}

	// Check if this is an explicit offer save action
	action := r.FormValue("action")
	isExplicitOfferSave := action == "save_offer" || action == "mark_offer_sent"

	// Check if offer already exists (existingDetail loaded above)
	existingOffer := existingDetail.Offer
	if existingOffer != nil {
		h.cfg.Debugf("  💰 Existing offer found: FinalPrice.Valid=%v, FinalPrice.Int32=%d, leadID=%s",
			existingOffer.FinalPrice.Valid, func() int32 {
				if existingOffer.FinalPrice.Valid {
					return existingOffer.FinalPrice.Int32
				}
				return 0
			}(), leadID)
	}

	// Process offer ONLY if:
	// 1. Bundle is explicitly provided (not empty), OR
	// 2. Explicit offer save action is triggered, OR
	// 3. Final price is explicitly provided (user manually set it)
	// Do NOT process if only existing offer exists (prevents auto-updating when saving other sections)
	paymentDealProvided := paymentBundleStr != "" || paymentDiscountAmountStr != "" || paymentFinalPriceStr != ""
	shouldProcessOffer := bundleStr != "" || isExplicitOfferSave || finalPriceStr != "" || paymentDealProvided
	h.cfg.Debugf("  💰 Offer processing check: bundleStr=%q, finalPriceStr=%q, isExplicitOfferSave=%v, existingOffer!=nil=%v, shouldProcess=%v, leadID=%s",
		bundleStr, finalPriceStr, isExplicitOfferSave, existingOffer != nil, shouldProcessOffer, leadID)

	if shouldProcessOffer {
		offer := &models.Offer{LeadID: leadID}

		// If offer exists, start with existing values
		if existingOffer != nil {
			offer.BundleLevels = existingOffer.BundleLevels
			offer.BasePrice = existingOffer.BasePrice
			offer.DiscountValue = existingOffer.DiscountValue
			offer.DiscountType = existingOffer.DiscountType
			offer.FinalPrice = existingOffer.FinalPrice
		}

		bundlePrices := pricingTrackBundlePrices(pricingTrack)

		// Update with form values
		var basePrice int32 = 0
		if paymentBundleStr != "" {
			bundleStr = paymentBundleStr
		}
		if bundleStr != "" {
			if b, err := strconv.Atoi(bundleStr); err == nil && b >= 1 && b <= 4 {
				bundleLevel := int32(b)
				price, ok := bundlePrices[bundleLevel]
				if !ok {
					h.renderDetailWithError(w, r, leadID, fmt.Sprintf("Bundle %d is not available for %s track.", bundleLevel, pricingTrack))
					return
				}
				offer.BundleLevels = sql.NullInt32{Int32: bundleLevel, Valid: true}
				basePrice = price
				offer.BasePrice = sql.NullInt32{Int32: basePrice, Valid: true}
				h.cfg.Debugf("  💰 %s bundle %d selected: auto-set base_price=%d, leadID=%s", pricingTrack, bundleLevel, basePrice, leadID)
			}
		}

		// If base price was set from bundle, use it; otherwise use form value
		if basePrice == 0 && basePriceStr != "" {
			if bp, err := strconv.Atoi(basePriceStr); err == nil {
				basePrice = int32(bp)
				offer.BasePrice = sql.NullInt32{Int32: basePrice, Valid: true}
			}
		}

		// If base price exists from existing offer and wasn't set above, use it
		if basePrice == 0 && existingOffer != nil && existingOffer.BasePrice.Valid {
			basePrice = existingOffer.BasePrice.Int32
		}

		// Parse discount (could be "500" or "10%")
		var discountAmount int32 = 0
		if paymentDiscountAmountStr != "" {
			discountStr = paymentDiscountAmountStr
			if paymentDiscountType == "percent" {
				discountStr = fmt.Sprintf("%s%%", paymentDiscountAmountStr)
			}
		}
		if discountStr != "" {
			if strings.HasSuffix(discountStr, "%") {
				if pct, err := strconv.Atoi(strings.TrimSuffix(discountStr, "%")); err == nil && basePrice > 0 {
					discountAmount = (basePrice * int32(pct)) / 100
					offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
					offer.DiscountType = sql.NullString{String: "percent", Valid: true}
					h.cfg.Debugf("  💰 Discount: %d%% = %d EGP (from base %d), leadID=%s", pct, discountAmount, basePrice, leadID)
				}
			} else {
				if amt, err := strconv.Atoi(discountStr); err == nil {
					discountAmount = int32(amt)
					offer.DiscountValue = sql.NullInt32{Int32: discountAmount, Valid: true}
					offer.DiscountType = sql.NullString{String: "amount", Valid: true}
					h.cfg.Debugf("  💰 Discount: %d EGP, leadID=%s", discountAmount, leadID)
				}
			}
		}

		// Calculate final price: base - discount (if base price is set)
		// PRIORITY: If final_price is explicitly provided, use it (highest priority)
		// Otherwise, calculate from base - discount
		// Otherwise, preserve existing
		if paymentFinalPriceStr != "" {
			finalPriceStr = paymentFinalPriceStr
		}
		if finalPriceStr != "" {
			if fp, err := strconv.Atoi(finalPriceStr); err == nil {
				offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
				h.cfg.Debugf("  💰 Offer Final Price: EXPLICIT from form=%d, leadID=%s", fp, leadID)
			} else {
				h.cfg.Debugf("  ⚠️  Failed to parse final_price: %q, error: %v, leadID=%s", finalPriceStr, err, leadID)
			}
		} else if basePrice > 0 {
			// Auto-calculate final price from base - discount (only if final_price not explicitly provided)
			calculatedFinalPrice := basePrice - discountAmount
			if calculatedFinalPrice < 0 {
				calculatedFinalPrice = 0
			}
			offer.FinalPrice = sql.NullInt32{Int32: calculatedFinalPrice, Valid: true}
			h.cfg.Debugf("  💰 Offer Final Price: AUTO-CALCULATED=%d (base=%d - discount=%d), leadID=%s",
				calculatedFinalPrice, basePrice, discountAmount, leadID)
		} else if existingOffer != nil && existingOffer.FinalPrice.Valid {
			// Preserve existing final price if not provided in form and can't calculate
			offer.FinalPrice = existingOffer.FinalPrice
			h.cfg.Debugf("  💰 Offer Final Price: PRESERVED existing=%d, leadID=%s", existingOffer.FinalPrice.Int32, leadID)
		} else {
			// No final price set - this is OK if it's a new offer
			h.cfg.Debugf("  ⚠️  Offer Final Price: NOT SET (new offer or no existing), leadID=%s", leadID)
		}

		detail.Offer = offer
		finalPriceVal := int32(0)
		if offer.FinalPrice.Valid {
			finalPriceVal = offer.FinalPrice.Int32
		}
		if offer.FinalPrice.Valid && offer.FinalPrice.Int32 == 0 {
			if existingOffer == nil || !existingOffer.FinalPrice.Valid || existingOffer.FinalPrice.Int32 != 0 || isExplicitOfferSave {
				if !canApproveZeroValueOffer(userRole) {
					h.renderDetailWithError(w, r, leadID, "Only Manager can approve a zero-value offer.")
					return
				}
			}
		}
		h.cfg.Debugf("  💰 Offer prepared for save: FinalPrice.Valid=%v, FinalPrice.Int32=%d, leadID=%s",
			offer.FinalPrice.Valid, finalPriceVal, leadID)
	} else {
		h.cfg.Debugf("  ⚠️  Offer NOT processed: no fields provided and no existing offer, leadID=%s", leadID)
	}

	// Booking
	bookFormat := strings.ToLower(strings.TrimSpace(r.FormValue("book_format")))
	if bookFormat != "" {
		if bookFormat != "pdf" && bookFormat != "printed" {
			h.renderDetailWithError(w, r, leadID, "Invalid book format. Allowed values are PDF or Printed.")
			return
		}

		booking := &models.Booking{
			LeadID:     leadID,
			BookFormat: sql.NullString{String: bookFormat, Valid: true},
		}
		if bookFormat == "pdf" {
			// Clear printed-only fields when PDF is selected.
			booking.Address = sql.NullString{Valid: false}
			booking.City = sql.NullString{Valid: false}
			booking.DeliveryNotes = sql.NullString{Valid: false}

			// Also clear shipping details when PDF is selected.
			detail.Shipping = &models.Shipping{
				LeadID:         leadID,
				ShipmentStatus: sql.NullString{Valid: false},
				ShipmentDate:   sql.NullTime{Valid: false},
			}
		} else {
			if address := strings.TrimSpace(r.FormValue("address")); address != "" {
				booking.Address = sql.NullString{String: address, Valid: true}
			}
			if city := strings.TrimSpace(r.FormValue("city")); city != "" {
				booking.City = sql.NullString{String: city, Valid: true}
			}
			if deliveryNotes := strings.TrimSpace(r.FormValue("delivery_notes")); deliveryNotes != "" {
				booking.DeliveryNotes = sql.NullString{String: deliveryNotes, Valid: true}
			}
		}
		detail.Booking = booking

		// Persist Booking & Materials immediately so this panel remains saved even if
		// unrelated sections fail later validation in the same submit.
		if err := models.UpsertBookingAndShipping(booking, detail.Shipping); err != nil {
			http.Error(w, "Couldn't save booking/materials. Please try again.", http.StatusInternalServerError)
			return
		}
	}

	// Payment (legacy Payment model - still used for display)
	var amountPaidValue int32 = 0
	if r.FormValue("payment_type") != "" || r.FormValue("amount_paid") != "" {
		payment := &models.Payment{LeadID: leadID}
		if paymentType := r.FormValue("payment_type"); paymentType != "" {
			payment.PaymentType = sql.NullString{String: paymentType, Valid: true}
		}
		if amountPaid := r.FormValue("amount_paid"); amountPaid != "" {
			if ap, err := strconv.Atoi(amountPaid); err == nil {
				amountPaidValue = int32(ap)
				payment.AmountPaid = sql.NullInt32{Int32: amountPaidValue, Valid: true}
			}
		}
		if remainingBalance := r.FormValue("remaining_balance"); remainingBalance != "" {
			if rb, err := strconv.Atoi(remainingBalance); err == nil {
				payment.RemainingBalance = sql.NullInt32{Int32: int32(rb), Valid: true}
			}
		}
		if paymentDate := r.FormValue("payment_date"); paymentDate != "" {
			if t, err := time.Parse("2006-01-02", paymentDate); err == nil {
				payment.PaymentDate = sql.NullTime{Time: t, Valid: true}
			}
		}
		detail.Payment = payment
	}

	// Course payment (new LeadPayment model for multiple payments)
	// Parse course payment fields if provided
	coursePaymentSourceRaw := strings.TrimSpace(r.FormValue("course_payment_source"))
	coursePaymentSource := coursePaymentSourceRaw
	if coursePaymentSource == "" {
		coursePaymentSource = "new_payment"
	}
	coursePaymentTransferIDStr := strings.TrimSpace(r.FormValue("course_payment_unidentified_transfer_id"))
	coursePaymentType := firstNonEmpty(r.FormValue("course_payment_type"), r.FormValue("payment_type"))
	coursePaymentAmountStr := firstNonEmpty(r.FormValue("payment_amount"), r.FormValue("course_payment_amount"))
	coursePaymentMethod := strings.TrimSpace(r.FormValue("course_payment_method"))
	coursePaymentDateStr := strings.TrimSpace(r.FormValue("course_payment_date"))
	coursePaymentNotes := r.FormValue("course_payment_notes")
	coursePaymentEnabled, coursePaymentLockReason := canUseCoursePaymentFlow(existingDetail)
	courseOfferFinalPrice := int32(0)
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
		courseOfferFinalPrice = detail.Offer.FinalPrice.Int32
	} else if existingDetail.Offer != nil && existingDetail.Offer.FinalPrice.Valid {
		courseOfferFinalPrice = existingDetail.Offer.FinalPrice.Int32
	}
	// Treat course payment as "intentional input" only when explicit payment selectors are touched.
	// Amount/date may be auto-filled by UI pricing helpers and must not, by themselves, trigger validation.
	coursePaymentFieldsTouched := coursePaymentType != "" || coursePaymentMethod != "" || coursePaymentSourceRaw == "unidentified_transfer" || coursePaymentTransferIDStr != ""
	if coursePaymentFieldsTouched && !coursePaymentEnabled {
		h.renderDetailWithError(w, r, leadID, coursePaymentLockReason)
		return
	}
	if coursePaymentFieldsTouched {
		allowedSources := map[string]bool{
			"new_payment":           true,
			"unidentified_transfer": true,
		}
		if !allowedSources[coursePaymentSource] {
			h.renderDetailWithErrorAndPaymentContext(
				w,
				r,
				leadID,
				"Invalid payment source selected.",
				map[string]string{
					"source":                   coursePaymentSourceRaw,
					"type":                     coursePaymentType,
					"amount":                   coursePaymentAmountStr,
					"method":                   coursePaymentMethod,
					"date":                     coursePaymentDateStr,
					"notes":                    coursePaymentNotes,
					"unidentified_transfer_id": coursePaymentTransferIDStr,
				},
				map[string]string{"source": "Choose a valid payment source."},
			)
			return
		}
	}
	if coursePaymentFieldsTouched && courseOfferFinalPrice == 0 {
		if detail.Offer == nil || !detail.Offer.FinalPrice.Valid {
			h.renderDetailWithErrorAndPaymentContext(
				w,
				r,
				leadID,
				"Cannot save a course payment because this lead's offer final price is missing. Save the offer first, then record the payment.",
				map[string]string{
					"source":                   coursePaymentSource,
					"type":                     coursePaymentType,
					"amount":                   coursePaymentAmountStr,
					"method":                   coursePaymentMethod,
					"date":                     coursePaymentDateStr,
					"notes":                    coursePaymentNotes,
					"unidentified_transfer_id": coursePaymentTransferIDStr,
				},
				nil,
			)
			return
		}
		h.renderDetailWithErrorAndPaymentContext(
			w,
			r,
			leadID,
			"This offer is zero-value, so no course payment can be recorded for it.",
			map[string]string{
				"source":                   coursePaymentSource,
				"type":                     coursePaymentType,
				"amount":                   coursePaymentAmountStr,
				"method":                   coursePaymentMethod,
				"date":                     coursePaymentDateStr,
				"notes":                    coursePaymentNotes,
				"unidentified_transfer_id": coursePaymentTransferIDStr,
			},
			nil,
		)
		return
	}
	if coursePaymentFieldsTouched {
		missingFields := make([]string, 0, 5)
		fieldErrors := make(map[string]string)
		if coursePaymentType == "" {
			missingFields = append(missingFields, "Payment Type")
			fieldErrors["type"] = "Payment type is required."
		}
		if coursePaymentSource == "unidentified_transfer" {
			if coursePaymentTransferIDStr == "" {
				missingFields = append(missingFields, "Unidentified Transfer")
				fieldErrors["unidentified_transfer_id"] = "Choose the transfer you want to attach to this lead."
			}
		} else {
			if coursePaymentAmountStr == "" {
				missingFields = append(missingFields, "Amount")
				fieldErrors["amount"] = "Amount is required."
			}
			if coursePaymentMethod == "" {
				missingFields = append(missingFields, "Payment Method")
				fieldErrors["method"] = "Payment method is required."
			}
			if coursePaymentDateStr == "" {
				missingFields = append(missingFields, "Payment Date")
				fieldErrors["date"] = "Payment date is required."
			}
		}
		if len(missingFields) > 0 {
			h.renderDetailWithErrorAndPaymentContext(
				w,
				r,
				leadID,
				fmt.Sprintf(
					"To save a course payment, complete all required fields (%s), or clear them all if you are not adding a payment.",
					strings.Join(missingFields, ", "),
				),
				map[string]string{
					"source":                   coursePaymentSource,
					"type":                     coursePaymentType,
					"amount":                   coursePaymentAmountStr,
					"method":                   coursePaymentMethod,
					"date":                     coursePaymentDateStr,
					"notes":                    coursePaymentNotes,
					"unidentified_transfer_id": coursePaymentTransferIDStr,
				},
				fieldErrors,
			)
			return
		}
	}

	// Auto-move to WAITING when payment is recorded (only for admin, only if status is before WAITING)
	if amountPaidValue > 0 {
		currentStatus := detail.Lead.Status
		// Statuses that come before waiting_for_round in the workflow
		statusesBeforeWaiting := map[string]bool{
			"lead_created":      true,
			"test_booked":       true,
			"tested":            true,
			"offer_sent":        true,
			"booking_confirmed": true,
			"deposit_paid":      true,
		}

		if statusesBeforeWaiting[currentStatus] {
			oldStatus := currentStatus
			detail.Lead.Status = "waiting_for_round"
			h.cfg.Debugf("  💰 Payment recorded (AmountPaid=%d): Auto-moving status %s → waiting_for_round", amountPaidValue, oldStatus)
		} else {
			h.cfg.Debugf("  💰 Payment recorded (AmountPaid=%d): Status is %s (not before WAITING), keeping current status", amountPaidValue, currentStatus)
		}
	}

	// Scheduling - validate and process class days and time
	classDays := r.FormValue("class_days")
	classTime := r.FormValue("class_time")

	creditsRemainingSchedule := int32(0)
	if existingDetail.Lead.LevelsPurchasedTotal.Valid {
		creditsRemainingSchedule = existingDetail.Lead.LevelsPurchasedTotal.Int32
	}
	if existingDetail.Lead.LevelsConsumed.Valid {
		creditsRemainingSchedule -= existingDetail.Lead.LevelsConsumed.Int32
	}
	if creditsRemainingSchedule < 0 {
		creditsRemainingSchedule = 0
	}

	scheduleLocked := false
	if existingDetail.Lead.IsReturning {
		scheduleLocked = creditsRemainingSchedule <= 0
	} else {
		scheduleLocked = existingDetail.Lead.Status != "waiting_for_round"
	}

	scheduleChange := classDays != "" || classTime != ""
	// If schedule is locked and values are only defaults, don't treat as a schedule change.
	if scheduleLocked && scheduleChange {
		existingDays := ""
		existingTime := ""
		if existingDetail.Scheduling != nil && existingDetail.Scheduling.ClassDays.Valid {
			existingDays = existingDetail.Scheduling.ClassDays.String
		}
		if existingDetail.Scheduling != nil && existingDetail.Scheduling.ClassTime.Valid {
			existingTime = normalizeClassTime(existingDetail.Scheduling.ClassTime.String)
		}
		// If existing schedule is empty, fall back to last class schedule defaults.
		if existingDays == "" && existingTime == "" && existingDetail.Lead.IsReturning {
			if lastDays, lastTime, err := models.GetLatestClassSchedule(leadID); err == nil {
				if lastDays.Valid {
					existingDays = lastDays.String
				}
				if lastTime.Valid {
					existingTime = normalizeClassTime(lastTime.String)
				}
			}
		}
		if classDays == existingDays && normalizeClassTime(classTime) == existingTime {
			classDays = ""
			classTime = ""
			scheduleChange = false
		}
	}

	// If user is setting schedule (either field provided), validate payment first
	if scheduleChange {
		// Check if fully paid before allowing schedule updates
		var finalPriceValue int32 = 0
		if existingDetail.Offer != nil && existingDetail.Offer.FinalPrice.Valid {
			finalPriceValue = existingDetail.Offer.FinalPrice.Int32
		}

		// Allow returning students with remaining credits to set schedule even if not fully paid.
		creditsRemaining := int32(0)
		if existingDetail.Lead.LevelsPurchasedTotal.Valid {
			creditsRemaining = existingDetail.Lead.LevelsPurchasedTotal.Int32
		}
		if existingDetail.Lead.LevelsConsumed.Valid {
			creditsRemaining -= existingDetail.Lead.LevelsConsumed.Int32
		}
		if creditsRemaining < 0 {
			creditsRemaining = 0
		}

		if !detail.Lead.IsReturning || creditsRemaining <= 0 {
			var totalCoursePaid int32
			if detail.Lead.IsReturning {
				totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
			} else {
				totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
			}
			if err != nil {
				log.Printf("ERROR: Failed to get total course paid: %v", err)
				totalCoursePaid = 0
			}

			isFullyPaid := existingDetail.Offer != nil && existingDetail.Offer.FinalPrice.Valid && totalCoursePaid >= finalPriceValue

			if !isFullyPaid {
				h.renderDetailWithError(w, r, leadID, "Cannot schedule before full payment. Course must be fully paid before setting class days and time.")
				return
			}
		}

		// Both fields must be present when setting NEW schedule
		// But if one is already set, allow updating just the other one
		existingScheduling := existingDetail.Scheduling
		hasExistingSchedule := existingScheduling != nil && existingScheduling.ClassDays.Valid && existingScheduling.ClassTime.Valid

		if !hasExistingSchedule && (classDays == "" || classTime == "") {
			h.renderDetailWithError(w, r, leadID, "Both Class Days and Class Time are required when setting schedule.")
			return
		}
	}

	// Validate class days (if provided)
	if classDays != "" {
		allowedClassDays := map[string]bool{
			"Sun/Wed":  true,
			"Sat/Tues": true,
			"Mon/Thu":  true,
		}
		if !allowedClassDays[classDays] {
			log.Printf("ERROR: Invalid class_days value: %q", classDays)
			h.renderDetailWithError(w, r, leadID, "Invalid class days value. Allowed values: Sun/Wed, Sat/Tues, Mon/Thu")
			return
		}
	}

	// Validate class time (if provided)
	if classTime != "" {
		allowedClassTimes := map[string]bool{
			"07:30": true,
			"10:00": true,
		}
		if !allowedClassTimes[classTime] {
			log.Printf("ERROR: Invalid class_time value: %q", classTime)
			h.renderDetailWithError(w, r, leadID, "Invalid class time value. Allowed values: 07:30, 10:00")
			return
		}
	}

	// Create/update scheduling if class days or time is provided
	// Note: Auto-stage classification (below) will handle status upgrade to READY_TO_START when schedule is filled
	// IMPORTANT: Always preserve existing scheduling values if form fields are not provided (e.g., when disabled)
	if classDays != "" || classTime != "" {
		// Load existing scheduling to preserve values not in form
		// Note: existingDetail was already loaded earlier, but we need fresh data for scheduling
		existingSchedulingDetail, err := models.GetLeadByID(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to load existing detail for scheduling preservation: %v", err)
			existingSchedulingDetail = nil
		}

		scheduling := &models.Scheduling{LeadID: leadID}

		// Set class_days if provided, otherwise preserve existing
		if classDays != "" {
			scheduling.ClassDays = sql.NullString{String: classDays, Valid: true}
		} else if existingSchedulingDetail != nil && existingSchedulingDetail.Scheduling != nil {
			scheduling.ClassDays = existingSchedulingDetail.Scheduling.ClassDays
		}

		// Set class_time if provided, otherwise preserve existing
		if classTime != "" {
			scheduling.ClassTime = sql.NullString{String: classTime, Valid: true}
		} else if existingSchedulingDetail != nil && existingSchedulingDetail.Scheduling != nil {
			scheduling.ClassTime = existingSchedulingDetail.Scheduling.ClassTime
		}

		// Preserve existing expected_round, start_date, start_time, class_group_index if they exist
		if existingSchedulingDetail != nil && existingSchedulingDetail.Scheduling != nil {
			scheduling.ExpectedRound = existingSchedulingDetail.Scheduling.ExpectedRound
			scheduling.StartDate = existingSchedulingDetail.Scheduling.StartDate
			scheduling.StartTime = existingSchedulingDetail.Scheduling.StartTime
			scheduling.ClassGroupIndex = existingSchedulingDetail.Scheduling.ClassGroupIndex
		}

		detail.Scheduling = scheduling
	}

	// Shipping
	if bookFormat != "pdf" && r.FormValue("shipment_status") != "" {
		shipping := &models.Shipping{LeadID: leadID}
		if shipmentStatus := r.FormValue("shipment_status"); shipmentStatus != "" {
			shipping.ShipmentStatus = sql.NullString{String: shipmentStatus, Valid: true}
		}
		if shipmentDate := r.FormValue("shipment_date"); shipmentDate != "" {
			if t, err := time.Parse("2006-01-02", shipmentDate); err == nil {
				shipping.ShipmentDate = sql.NullTime{Time: t, Valid: true}
			}
		}
		detail.Shipping = shipping
	}

	// CRITICAL: Preserve existing offer for display/computation, but track if it was explicitly changed
	// This ensures that if user only updates other sections, offer final_price is not lost
	// BUT: We only use it for status upgrade if it was explicitly changed
	offerWasExplicitlyChanged := shouldProcessOffer
	if detail.Offer == nil && existingDetail.Offer != nil {
		detail.Offer = existingDetail.Offer
		h.cfg.Debugf("  💰 Preserving existing offer: FinalPrice.Valid=%v, FinalPrice.Int32=%d, leadID=%s",
			existingDetail.Offer.FinalPrice.Valid, func() int32 {
				if existingDetail.Offer.FinalPrice.Valid {
					return existingDetail.Offer.FinalPrice.Int32
				}
				return 0
			}(), leadID)
	}
	// Ensure we have existing payment data if form didn't modify it (for stage computation)
	if detail.Payment == nil && existingDetail.Payment != nil {
		detail.Payment = existingDetail.Payment
	}
	// Ensure we have existing scheduling data if form didn't modify it (for stage computation)
	if detail.Scheduling == nil && existingDetail.Scheduling != nil {
		detail.Scheduling = existingDetail.Scheduling
	}

	// Auto-compute stage from form completion and update status
	// This happens after all form sections are parsed
	// IMPORTANT: Only upgrade to OFFER_SENT if offer was explicitly changed
	// Create a copy of detail for stage computation, removing offer if it wasn't explicitly changed
	stageDetail := detail
	if !offerWasExplicitlyChanged && detail.Offer != nil {
		// Create a copy without the offer to prevent status upgrade
		stageDetailCopy := *detail
		stageDetailCopy.Offer = nil
		stageDetail = &stageDetailCopy
		h.cfg.Debugf("  📊 Stage computation: Offer NOT explicitly changed, excluding from status upgrade, leadID=%s", leadID)
	} else if offerWasExplicitlyChanged {
		h.cfg.Debugf("  📊 Stage computation: Offer WAS explicitly changed, including in status upgrade, leadID=%s", leadID)
	}
	if currentStatus == "cold_lead" {
		detail.Lead.Status = currentStatus
		h.cfg.Debugf("  🧊 Cold lead: preserving status, skipping auto-stage, leadID=%s", leadID)
	} else if currentStatus == "renewal_pending" && offerWasExplicitlyChanged {
		// Allow renewal_pending → offer_sent when offer is saved
		newStage, dbStatus := models.ComputeStageFromFormCompletion(stageDetail, currentStatus)
		detail.Lead.Status = dbStatus
		h.cfg.Debugf("  ♻️ Renewal offer changed: computed stage=%s, dbStatus=%s (was %s)", newStage, dbStatus, currentStatus)
	} else if currentStatus == "in_classes" {
		detail.Lead.Status = currentStatus
		h.cfg.Debugf("  🎯 In-classes lead: preserving status, skipping auto-stage, leadID=%s", leadID)
	} else if currentStatus == "renewal_pending" || currentStatus == "waiting_for_round" || detail.Lead.IsReturning {
		detail.Lead.Status = currentStatus
		h.cfg.Debugf("  ♻️ Returning lead: preserving status, skipping auto-stage, leadID=%s", leadID)
	} else {
		newStage, dbStatus := models.ComputeStageFromFormCompletion(stageDetail, currentStatus)

		// Validation: If stage reaches OFFER_SENT or later, final_price must be valid
		if newStage == models.StageOfferSent || newStage == models.StageBookingConfirmedPaidFull || newStage == models.StageBookingConfirmedDeposit {
			if detail.Offer == nil || !detail.Offer.FinalPrice.Valid || detail.Offer.FinalPrice.Int32 < 0 {
				h.renderDetailWithError(w, r, leadID, "Final price is required when sending an offer. Please fill in the Offer & Pricing section.")
				return
			}
		}

		detail.Lead.Status = dbStatus
		h.cfg.Debugf("  📊 Auto-stage: computed stage=%s, dbStatus=%s (was %s)", newStage, dbStatus, currentStatus)
	}

	// Log offer final price before saving
	if detail.Offer != nil {
		finalPriceVal := int32(0)
		if detail.Offer.FinalPrice.Valid {
			finalPriceVal = detail.Offer.FinalPrice.Int32
		}
		h.cfg.Debugf("  💾 About to save: Offer.FinalPrice.Valid=%v, Offer.FinalPrice.Int32=%d, leadID=%s",
			detail.Offer.FinalPrice.Valid, finalPriceVal, leadID)
	} else {
		h.cfg.Debugf("  💾 About to save: Offer is nil, leadID=%s", leadID)
	}

	err = models.UpdateLeadDetail(detail)
	if err != nil {
		// Check if it's a phone constraint error
		var phoneErr *models.PhoneAlreadyExistsError
		if errors.As(err, &phoneErr) {
			// phoneErr already has the details from UpdateLeadDetail
		} else if phoneConstraintErr := models.IsPhoneConstraintError(err); phoneConstraintErr != nil {
			// Try to get the full error with existing lead ID
			if existingLead, findErr := models.GetLeadByPhone(detail.Lead.Phone); findErr == nil && existingLead.ID != leadID {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:          detail.Lead.Phone,
					ExistingLeadID: &existingLead.ID,
					Message:        "Phone number already exists",
				}
			} else {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:   detail.Lead.Phone,
					Message: "Phone number already exists",
				}
			}
		}

		if phoneErr != nil {
			// Redirect back to detail page with error
			redirectURL := fmt.Sprintf("/pre-enrolment/%s?error=phone_exists", leadID.String())
			if phoneErr.ExistingLeadID != nil {
				redirectURL += fmt.Sprintf("&existing_lead_id=%s", phoneErr.ExistingLeadID.String())
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		http.Error(w, "Couldn't update this lead. Please try again.", http.StatusInternalServerError)
		return
	}

	// Log after saving - reload to verify
	reloadedDetail, err := models.GetLeadByID(leadID)
	if err == nil && reloadedDetail.Offer != nil {
		finalPriceVal := int32(0)
		if reloadedDetail.Offer.FinalPrice.Valid {
			finalPriceVal = reloadedDetail.Offer.FinalPrice.Int32
		}
		h.cfg.Debugf("  ✅ After save: Offer.FinalPrice.Valid=%v, Offer.FinalPrice.Int32=%d, leadID=%s",
			reloadedDetail.Offer.FinalPrice.Valid, finalPriceVal, leadID)
	} else if err == nil {
		h.cfg.Debugf("  ⚠️  After save: Offer is nil, leadID=%s", leadID)
	}

	// Sync finance transactions for placement test
	if detail.PlacementTest != nil {
		amountPaid := int32(0)
		if detail.PlacementTest.PlacementTestFeePaid.Valid {
			amountPaid = detail.PlacementTest.PlacementTestFeePaid.Int32
		}

		// Validate: if amount > 0, date and method must be provided
		if amountPaid > 0 {
			if !detail.PlacementTest.PlacementTestPaymentDate.Valid || !detail.PlacementTest.PlacementTestPaymentMethod.Valid {
				h.renderDetailWithError(w, r, leadID, "Payment date and method are required when placement test fee is paid.")
				return
			}
		}

		err = models.UpsertPlacementTestIncome(leadID, amountPaid, detail.PlacementTest.PlacementTestPaymentDate, detail.PlacementTest.PlacementTestPaymentMethod)
		if err != nil {
			log.Printf("ERROR: Failed to sync placement test finance transaction: %v", err)
			// Check if it's a validation error (future date)
			errorMsg := "Couldn't sync the placement test payment. Please try again."
			if err.Error() == "payment date cannot be in the future" {
				errorMsg = "Payment date cannot be in the future"
			}
			h.renderDetailWithError(w, r, leadID, errorMsg)
			return
		}
	}

	// Sync finance transactions for course payment (create new LeadPayment if provided)
	if coursePaymentFieldsTouched {
		// Re-evaluate status at payment time (currentStatus was captured at request start).
		// This avoids false blocks when status was legitimately advanced earlier in this save flow.
		statusForPaymentGate := currentStatus
		if detail != nil && detail.Lead != nil && strings.TrimSpace(detail.Lead.Status) != "" {
			statusForPaymentGate = detail.Lead.Status
		}
		if latestDetail, err := models.GetLeadByID(leadID); err == nil && latestDetail != nil {
			if strings.TrimSpace(latestDetail.Lead.Status) != "" {
				statusForPaymentGate = latestDetail.Lead.Status
			}
		}

		if !isStatusAtOrAfterOfferSent(statusForPaymentGate) {
			h.renderDetailWithError(w, r, leadID, "You must click 'Mark Offer Sent' before collecting payment.")
			return
		}

		// Validate payment type is provided
		if coursePaymentType == "" {
			h.renderDetailWithError(w, r, leadID, "Payment type is required (Deposit, Full Payment, or Top-up).")
			return
		}

		// Validate payment type value
		allowedPaymentTypes := map[string]bool{
			"deposit":      true,
			"full_payment": true,
			"top_up":       true,
		}
		if !allowedPaymentTypes[coursePaymentType] {
			h.renderDetailWithError(w, r, leadID, "Invalid payment type. Must be: deposit, full_payment, or top_up.")
			return
		}

		finalPriceValue := courseOfferFinalPrice
		if finalPriceValue <= 0 {
			h.renderDetailWithError(w, r, leadID, "Final offer amount must be set from bundle and discount before collecting payment.")
			return
		}

		// Resolve bundle levels for payment-cycle locking and credits update.
		var bundleLevels sql.NullInt32
		if paymentBundleStr != "" {
			if b, parseErr := strconv.Atoi(paymentBundleStr); parseErr == nil && b >= 1 && b <= 4 {
				bundleLevels = sql.NullInt32{Int32: int32(b), Valid: true}
			}
		}
		if !bundleLevels.Valid && detail.Offer != nil && detail.Offer.BundleLevels.Valid {
			bundleLevels = detail.Offer.BundleLevels
		}
		if !bundleLevels.Valid && existingDetail.Offer != nil && existingDetail.Offer.BundleLevels.Valid {
			bundleLevels = existingDetail.Offer.BundleLevels
		}
		if !bundleLevels.Valid && existingDetail.Lead.LevelsPurchasedTotal.Valid && existingDetail.Lead.LevelsPurchasedTotal.Int32 > 0 {
			bundleLevels = sql.NullInt32{Int32: existingDetail.Lead.LevelsPurchasedTotal.Int32, Valid: true}
		}

		// For returning-cycle payments, lock/initialize cycle BEFORE validation totals
		// so the first payment is counted in this cycle on subsequent attempts.
		if existingDetail.Lead.IsReturning {
			if !bundleLevels.Valid {
				h.renderDetailWithError(w, r, leadID, "Bundle selection is required before collecting renewal payment.")
				return
			}
			if err := models.UpsertActivePaymentCycle(leadID, bundleLevels.Int32, finalPriceValue); err != nil {
				log.Printf("ERROR: Failed to upsert active payment cycle (pre-payment): %v", err)
				h.renderDetailWithError(w, r, leadID, "Couldn't lock payment cycle safely. Please try again.")
				return
			}
		}

		// Get total course paid (current cycle only for returning students)
		var totalCoursePaid int32
		if existingDetail.Lead.IsReturning {
			totalCoursePaid, err = models.GetTotalCoursePaidCurrentCycle(leadID)
		} else {
			totalCoursePaid, err = models.GetTotalCoursePaid(leadID)
		}
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			h.renderDetailWithError(w, r, leadID, "Couldn't validate the course payment. Please try again.")
			return
		}

		amount := 0
		var paymentDate time.Time
		paymentMethod := coursePaymentMethod
		var transferID uuid.UUID
		if coursePaymentSource == "unidentified_transfer" {
			transferID, err = uuid.Parse(coursePaymentTransferIDStr)
			if err != nil {
				h.renderDetailWithErrorAndPaymentContext(
					w,
					r,
					leadID,
					"Choose a valid unidentified transfer before saving this payment.",
					map[string]string{
						"source":                   coursePaymentSource,
						"type":                     coursePaymentType,
						"amount":                   coursePaymentAmountStr,
						"method":                   coursePaymentMethod,
						"date":                     coursePaymentDateStr,
						"notes":                    coursePaymentNotes,
						"unidentified_transfer_id": coursePaymentTransferIDStr,
					},
					map[string]string{"unidentified_transfer_id": "Choose a valid transfer."},
				)
				return
			}

			availableTransfers, loadErr := models.GetUnidentifiedTransfers()
			if loadErr != nil {
				log.Printf("ERROR: Failed to load unidentified transfers for reconciliation: %v", loadErr)
				h.renderDetailWithError(w, r, leadID, "Couldn't load the unidentified transfer. Please refresh and try again.")
				return
			}
			var selectedTransfer *models.Transaction
			for _, candidate := range availableTransfers {
				if candidate.ID == transferID {
					selectedTransfer = candidate
					break
				}
			}
			if selectedTransfer == nil {
				h.renderDetailWithErrorAndPaymentContext(
					w,
					r,
					leadID,
					"That unidentified transfer is no longer available. Refresh the page and choose another one.",
					map[string]string{
						"source":                   coursePaymentSource,
						"type":                     coursePaymentType,
						"amount":                   coursePaymentAmountStr,
						"method":                   coursePaymentMethod,
						"date":                     coursePaymentDateStr,
						"notes":                    coursePaymentNotes,
						"unidentified_transfer_id": coursePaymentTransferIDStr,
					},
					map[string]string{"unidentified_transfer_id": "This transfer was already matched or removed."},
				)
				return
			}
			amount = int(selectedTransfer.Amount)
			paymentDate = selectedTransfer.TransactionDate
			if selectedTransfer.PaymentMethod.Valid {
				paymentMethod = selectedTransfer.PaymentMethod.String
			}
		} else {
			amount, err = strconv.Atoi(coursePaymentAmountStr)
			if err != nil || amount <= 0 {
				h.renderDetailWithError(w, r, leadID, "Invalid course payment amount.")
				return
			}
			paymentDate, err = util.ParseDateLocal(coursePaymentDateStr)
			if err != nil {
				h.renderDetailWithError(w, r, leadID, "Invalid course payment date.")
				return
			}
		}

		if coursePaymentType == "full_payment" && int32(amount) != finalPriceValue {
			h.renderDetailWithError(w, r, leadID, fmt.Sprintf("For Full payment, amount must equal final due (%d EGP).", finalPriceValue))
			return
		}
		remainingBalance := finalPriceValue - totalCoursePaid
		if remainingBalance < 0 {
			remainingBalance = 0
		}
		if remainingBalance == 0 {
			h.renderDetailWithError(w, r, leadID, "Course is already fully paid for this cycle. Additional payments are not allowed.")
			return
		}
		if int32(amount) > remainingBalance {
			h.renderDetailWithError(w, r, leadID, fmt.Sprintf("Course payment amount (%d) exceeds remaining balance (%d). Total course paid cannot exceed offer final price.", amount, remainingBalance))
			return
		}
		if totalCoursePaid+int32(amount) > finalPriceValue {
			h.renderDetailWithError(w, r, leadID, "Total course paid cannot exceed offer final price.")
			return
		}

		if coursePaymentSource == "unidentified_transfer" {
			var reconciledBy *uuid.UUID
			if userIDStr := strings.TrimSpace(middleware.GetUserID(r)); userIDStr != "" {
				if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
					reconciledBy = &parsed
				}
			}
			_, err = models.ReconcileUnidentifiedTransferToLead(transferID, leadID, coursePaymentType, coursePaymentNotes, reconciledBy)
		} else {
			_, err = models.CreateLeadPayment(leadID, coursePaymentType, int32(amount), paymentMethod, paymentDate, coursePaymentNotes)
		}
		if err != nil {
			log.Printf("ERROR: Failed to create course payment: %v", err)
			// Check if it's a validation error (future date)
			errorMsg := "Couldn't create the course payment. Please try again."
			if err.Error() == "payment date cannot be in the future" {
				errorMsg = "Payment date cannot be in the future"
			} else if err.Error() == "this transfer is no longer available for reconciliation" || err.Error() == "unidentified transfer not found" {
				errorMsg = "That unidentified transfer is no longer available. Refresh the page and try again."
			}
			h.renderDetailWithError(w, r, leadID, errorMsg)
			return
		}

		// Update lead credits based on all payments.
		err = models.UpdateLeadCreditsFromPayments(leadID, bundleLevels)
		if err != nil {
			log.Printf("ERROR: Failed to update lead credits: %v", err)
			// Don't fail the save, just log
		}
	}

	if returnToListAfterSave || existingDetail.Lead.Status == "lead_created" {
		http.Redirect(w, r, "/pre-enrolment?saved=1", http.StatusFound)
		return
	}

	// Redirect back to detail page to show saved changes
	http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?saved=1", leadID.String()), http.StatusFound)
}

// MarkTested sets status to "tested" and optionally saves test notes/assigned level
func (h *PreEnrolmentHandler) MarkTested(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	// Update placement test if fields are provided
	if r.FormValue("assigned_level") != "" || r.FormValue("test_notes") != "" {
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
			return
		}

		if detail.PlacementTest == nil {
			detail.PlacementTest = &models.PlacementTest{LeadID: leadID}
		}

		if assignedLevel := r.FormValue("assigned_level"); assignedLevel != "" {
			level, parseErr := strconv.Atoi(assignedLevel)
			if parseErr != nil || !isValidAssignedLevel(level) {
				h.renderDetailWithError(w, r, leadID, "Invalid assigned level. Allowed: 1–10.")
				return
			}
			detail.PlacementTest.AssignedLevel = sql.NullInt32{Int32: int32(level), Valid: true}
		}
		if testNotes := r.FormValue("test_notes"); testNotes != "" {
			detail.PlacementTest.TestNotes = sql.NullString{String: testNotes, Valid: true}
		}

		// Update placement test only
		if err := models.UpdatePlacementTest(detail.PlacementTest); err != nil {
			http.Error(w, "Couldn't update the placement test. Please try again.", http.StatusInternalServerError)
			return
		}
	}

	// Update status
	err = models.UpdateLeadStatus(leadID, "tested")
	if err != nil {
		http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=tested", http.StatusFound)
}

// MarkOfferSent sets status to "offer_sent" and validates offer fields
func (h *PreEnrolmentHandler) MarkOfferSent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	// Bundle is now optional - OP Admin can send all bundle options without pre-selecting
	// Only validate consistency: if bundle is selected, final_price should be set, and vice versa
	bundle := r.FormValue("bundle")
	finalPrice := r.FormValue("final_price")

	// If bundle is selected, final_price must be set
	if bundle != "" && finalPrice == "" {
		h.renderDetailWithError(w, r, leadID, "Please set Final Price for the selected bundle.")
		return
	}

	// If final_price is set, bundle must be selected
	if finalPrice != "" && bundle == "" {
		h.renderDetailWithError(w, r, leadID, "Please select a bundle for the specified price.")
		return
	}

	// Both can be empty - means OP Admin is sending all options to student

	// Update or create offer
	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
		return
	}
	if allowed, reason := canMarkOfferSent(detail); !allowed {
		h.renderDetailWithError(w, r, leadID, reason)
		return
	}

	// Persist Booking & Materials in the same submit (important for "Packages Sent" flow).
	bookFormat := strings.ToLower(strings.TrimSpace(r.FormValue("book_format")))
	if bookFormat != "" {
		if bookFormat != "pdf" && bookFormat != "printed" {
			h.renderDetailWithError(w, r, leadID, "Invalid book format. Allowed values are PDF or Printed.")
			return
		}

		booking := &models.Booking{
			LeadID:     leadID,
			BookFormat: sql.NullString{String: bookFormat, Valid: true},
		}
		var shipping *models.Shipping

		if bookFormat == "pdf" {
			booking.Address = sql.NullString{Valid: false}
			booking.City = sql.NullString{Valid: false}
			booking.DeliveryNotes = sql.NullString{Valid: false}
			shipping = &models.Shipping{
				LeadID:         leadID,
				ShipmentStatus: sql.NullString{Valid: false},
				ShipmentDate:   sql.NullTime{Valid: false},
			}
		} else {
			if address := strings.TrimSpace(r.FormValue("address")); address != "" {
				booking.Address = sql.NullString{String: address, Valid: true}
			}
			if city := strings.TrimSpace(r.FormValue("city")); city != "" {
				booking.City = sql.NullString{String: city, Valid: true}
			}
			if notes := strings.TrimSpace(r.FormValue("delivery_notes")); notes != "" {
				booking.DeliveryNotes = sql.NullString{String: notes, Valid: true}
			}
		}

		if err := models.UpsertBookingAndShipping(booking, shipping); err != nil {
			http.Error(w, "Couldn't save booking/materials. Please try again.", http.StatusInternalServerError)
			return
		}
	}

	if detail.Offer == nil {
		detail.Offer = &models.Offer{LeadID: leadID}
	}
	if offerNotes := strings.TrimSpace(r.FormValue("offer_notes")); offerNotes != "" {
		detail.Offer.FollowUpNotes = sql.NullString{String: offerNotes, Valid: true}
	} else {
		detail.Offer.FollowUpNotes = sql.NullString{Valid: false}
	}

	if b, err := strconv.Atoi(bundle); err == nil {
		detail.Offer.BundleLevels = sql.NullInt32{Int32: int32(b), Valid: true}
	}
	if fp, err := strconv.Atoi(finalPrice); err == nil {
		detail.Offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
	}
	if basePrice := r.FormValue("base_price"); basePrice != "" {
		if bp, err := strconv.Atoi(basePrice); err == nil {
			detail.Offer.BasePrice = sql.NullInt32{Int32: int32(bp), Valid: true}
		}
	}
	if discount := r.FormValue("discount"); discount != "" {
		// Parse discount (could be "500" or "10%")
		if strings.HasSuffix(discount, "%") {
			if pct, err := strconv.Atoi(strings.TrimSuffix(discount, "%")); err == nil {
				detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
				detail.Offer.DiscountType = sql.NullString{String: "percent", Valid: true}
			}
		} else {
			if amt, err := strconv.Atoi(discount); err == nil {
				detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(amt), Valid: true}
				detail.Offer.DiscountType = sql.NullString{String: "amount", Valid: true}
			}
		}
	}

	// Update offer
	if err := models.UpdateOffer(detail.Offer); err != nil {
		http.Error(w, "Couldn't update the offer. Please try again.", http.StatusInternalServerError)
		return
	}

	// Update status
	err = models.UpdateLeadStatus(leadID, "offer_sent")
	if err != nil {
		http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=offer_sent", http.StatusFound)
}

// MarkWaiting sets status to "waiting_for_round"
func (h *PreEnrolmentHandler) MarkWaiting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
		return
	}
	if !canUseWaitingFlow(detail) {
		h.renderDetailWithError(w, r, leadID, "Cannot move to waiting list: this lead must be fully paid or prepaid for the next round first.")
		return
	}

	err = models.UpdateLeadStatus(leadID, "waiting_for_round")
	if err != nil {
		http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=waiting", http.StatusFound)
}

// MarkReady sets status to "ready_to_start"
func (h *PreEnrolmentHandler) MarkReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "You don't have permission to update this lead.", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	err = models.UpdateLeadStatus(leadID, "ready_to_start")
	if err != nil {
		http.Error(w, "Couldn't update the status. Please try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=ready", http.StatusFound)
}

func (h *PreEnrolmentHandler) BookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot book tests
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "You don't have permission to book placement tests.", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "We couldn't find that lead. Please refresh and try again.", http.StatusBadRequest)
		return
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, "Couldn't load this lead. Please refresh and try again.", http.StatusInternalServerError)
		return
	}
	if isReturningCyclePlacementLocked(detail.Lead) {
		http.Error(w, "Placement test booking is locked for returning students. Continue with renewal payment flow.", http.StatusBadRequest)
		return
	}

	placementTest, err := buildBookedPlacementTestFromRequest(leadID, detail.PlacementTest, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.cfg.Debugf("📅 BookTest: leadID=%s, testDate=%v, testTime=%v, testType=%v", leadID, placementTest.TestDate, placementTest.TestTime, placementTest.TestType)

	// Book the placement test (updates test fields and sets status to test_booked).
	err = models.BookPlacementTest(leadID, placementTest)
	if err != nil {
		log.Printf("ERROR: Failed to book placement test: %v", err)
		http.Error(w, "Couldn't book the placement test. Please try again.", http.StatusInternalServerError)
		return
	}
	if err := syncPlacementTestFinanceForBooking(leadID, placementTest); err != nil {
		log.Printf("ERROR: Failed to sync placement test finance transaction: %v", err)
		http.Error(w, "Couldn't sync the placement test payment. Please try again.", http.StatusInternalServerError)
		return
	}

	h.cfg.Debugf("  ✅ Test booked successfully, redirecting to list")
	http.Redirect(w, r, "/pre-enrolment?status_flash=test_booked", http.StatusFound)
}
