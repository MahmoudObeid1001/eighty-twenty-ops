package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"eighty-twenty-ops/internal/models"
)

var smartStepArabicTemplates = map[string]string{
	"CONFIRM_REPEAT_LEVEL":     "الطالب مكرر مستوى، ثبّت نفس المستوى في الخطة.",
	"BOOK_TEST":                "احجز اختبار تحديد المستوى.",
	"RUN_TEST_SET_LEVEL":       "نفّذ الاختبار وحدّد المستوى المناسب.",
	"SEND_OFFER":               "أرسل الباقات المناسبة حسب المستوى.",
	"CREATE_RENEWAL_OFFER":     "أنشئ عرض تجديد جديد لهذا الطالب.",
	"COLLECT_RENEWAL_PAYMENT":  "سجّل دفعة التجديد المطلوبة.",
	"FOLLOW_UP_PAYMENT":        "تابع مع الطالب لإتمام الدفع.",
	"SET_SCHEDULE":             "حدّد أيام ووقت الحصة.",
	"MARK_READY":               "بعد التأكد من الشروط، انقل الحالة إلى جاهز للبداية.",
	"SEND_TO_CLASSES":          "أرسل الطالب إلى الكلاسات.",
	"SEND_TO_CLASS_OPTIONS":    "الطالب مدفوع بالكامل: أرسله للكلاس، أو انقله لقائمة الانتظار، أو أضفه Late Joiner حسب الحالة.",
	"TRACK_CLASS_PROGRESS":     "تابع حضور الطالب وتقدمه داخل الجولة.",
	"RETARGET_REFUSED_RENEWAL": "الطالب رفض التجديد حالياً: أضفه لخطة إعادة الاستهداف من قسم Cold Leads.",
	"REVIEW_DATA":              "راجع بيانات الطالب الأساسية قبل المتابعة.",
}

func appendStepCodeUnique(out []string, code string) []string {
	code = strings.TrimSpace(code)
	if code == "" {
		return out
	}
	for _, existing := range out {
		if existing == code {
			return out
		}
	}
	return append(out, code)
}

func deterministicSmartStepCodes(detail *models.LeadDetail, isFullyPaid bool, creditsRemaining int32, finalPriceValue, totalCoursePaid int32, lastOutcome string) []string {
	if detail == nil || detail.Lead == nil {
		return []string{"REVIEW_DATA"}
	}

	status := strings.TrimSpace(detail.Lead.Status)
	hasLevel := detail.PlacementTest != nil && detail.PlacementTest.AssignedLevel.Valid
	hasOffer := detail.Offer != nil && detail.Offer.FinalPrice.Valid && detail.Offer.FinalPrice.Int32 > 0
	scheduleReady := detail.Scheduling != nil && detail.Scheduling.ClassDays.Valid && detail.Scheduling.ClassTime.Valid
	balanceDue := finalPriceValue > 0 && totalCoursePaid < finalPriceValue

	steps := make([]string, 0, 4)
	if strings.EqualFold(lastOutcome, "repeated") {
		steps = appendStepCodeUnique(steps, "CONFIRM_REPEAT_LEVEL")
	}

	switch status {
	case "lead_created":
		steps = appendStepCodeUnique(steps, "BOOK_TEST")
	case "test_booked":
		steps = appendStepCodeUnique(steps, "RUN_TEST_SET_LEVEL")
	case "tested":
		steps = appendStepCodeUnique(steps, "SEND_OFFER")
	case "offer_sent":
		if !isFullyPaid || balanceDue {
			steps = appendStepCodeUnique(steps, "FOLLOW_UP_PAYMENT")
		}
		if hasLevel && isFullyPaid && !scheduleReady {
			steps = appendStepCodeUnique(steps, "SET_SCHEDULE")
		}
		if isFullyPaid && scheduleReady {
			steps = appendStepCodeUnique(steps, "MARK_READY")
		}
	case "renewal_pending":
		if creditsRemaining <= 0 {
			if !hasOffer {
				steps = appendStepCodeUnique(steps, "CREATE_RENEWAL_OFFER")
			}
			if hasOffer && (!isFullyPaid || balanceDue) {
				steps = appendStepCodeUnique(steps, "COLLECT_RENEWAL_PAYMENT")
			}
		}
		if (isFullyPaid || creditsRemaining > 0) && !scheduleReady {
			steps = appendStepCodeUnique(steps, "SET_SCHEDULE")
		}
		if (isFullyPaid || creditsRemaining > 0) && scheduleReady {
			steps = appendStepCodeUnique(steps, "MARK_READY")
		}
	case "waiting_for_round":
		if !scheduleReady {
			steps = appendStepCodeUnique(steps, "SET_SCHEDULE")
		} else {
			steps = appendStepCodeUnique(steps, "MARK_READY")
		}
	case "schedule_assigned":
		steps = appendStepCodeUnique(steps, "MARK_READY")
	case "ready_to_start":
		steps = appendStepCodeUnique(steps, "SEND_TO_CLASS_OPTIONS")
	case "in_classes":
		steps = appendStepCodeUnique(steps, "TRACK_CLASS_PROGRESS")
	case "cold_lead":
		if detail.Lead.IsReturning && creditsRemaining <= 0 {
			steps = appendStepCodeUnique(steps, "RETARGET_REFUSED_RENEWAL")
		} else {
			steps = appendStepCodeUnique(steps, "REVIEW_DATA")
		}
	case "paid_full", "booking_confirmed":
		if !scheduleReady {
			steps = appendStepCodeUnique(steps, "SET_SCHEDULE")
		} else {
			steps = appendStepCodeUnique(steps, "SEND_TO_CLASS_OPTIONS")
		}
	case "deposit_paid":
		if hasOffer {
			steps = appendStepCodeUnique(steps, "FOLLOW_UP_PAYMENT")
		} else {
			steps = appendStepCodeUnique(steps, "SEND_OFFER")
		}
	default:
		if isFullyPaid && hasLevel {
			if !scheduleReady {
				steps = appendStepCodeUnique(steps, "SET_SCHEDULE")
			} else {
				steps = appendStepCodeUnique(steps, "SEND_TO_CLASS_OPTIONS")
			}
		} else {
			steps = appendStepCodeUnique(steps, "REVIEW_DATA")
		}
	}

	if len(steps) == 0 {
		steps = append(steps, "REVIEW_DATA")
	}
	return steps
}

func translateSmartStepCodesToArabic(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		if v, ok := smartStepArabicTemplates[code]; ok {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return []string{smartStepArabicTemplates["REVIEW_DATA"]}
	}
	return out
}

func sleepingLeadSmartSteps(item *models.LeadListItem) ([]string, []string, string) {
	if item == nil {
		return nil, nil, ""
	}

	if item.SleepingReminderAt.Valid {
		if item.SleepingReminderDue {
			steps := []string{"راجع الطالب اليوم لأن موعد التذكير المؤجل حان."}
			if item.SleepingReminderNote.Valid && strings.TrimSpace(item.SleepingReminderNote.String) != "" {
				steps = append(steps, "ملاحظة التذكير: "+strings.TrimSpace(item.SleepingReminderNote.String))
			}
			steps = append(steps, "حدّث الحالة أو جدّد التذكير إذا طلب الطالب موعداً آخر.")
			return []string{"SLEEPING_REMINDER_DUE"}, steps, "template"
		}

		steps := []string{fmt.Sprintf("تم تأجيل المتابعة إلى %s.", item.SleepingReminderAt.Time.Format("2006-01-02"))}
		if item.SleepingReminderNote.Valid && strings.TrimSpace(item.SleepingReminderNote.String) != "" {
			steps = append(steps, "ملاحظة التذكير: "+strings.TrimSpace(item.SleepingReminderNote.String))
		}
		return []string{"SLEEPING_REMINDER_SET"}, steps, "template"
	}

	currentStep := item.SleepingLeadStep
	if currentStep <= 0 {
		currentStep = 1
	}

	steps := make([]string, 0, 2)
	switch {
	case currentStep == 1:
		steps = append(steps, "أرسل رسالة المتابعة الأولى للطالب عبر واتساب.")
	case currentStep == 2:
		steps = append(steps, "أرسل رسالة المتابعة الثانية للطالب عبر واتساب.")
	case currentStep == 3:
		steps = append(steps, "أرسل رسالة المتابعة الثالثة والأخيرة للطالب عبر واتساب.")
	default:
		steps = append(steps, "سلسلة رسائل Sleeping Leads اكتملت. غيّر الحالة عند الرد أو التقدّم.")
	}

	if currentStep > 1 && currentStep <= 3 {
		steps = append(steps, fmt.Sprintf("تم تسجيل الرسالة %d مسبقاً لهذا الطالب.", currentStep-1))
	}

	return []string{"SLEEPING_SEQUENCE"}, steps, "template"
}

func sanitizeJSONText(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func (h *PreEnrolmentHandler) rewriteSmartStepsArabic(codes []string, templateSteps []string, detail *models.LeadDetail) ([]string, bool, error) {
	if !h.cfg.SmartStepsAIEnabled || strings.TrimSpace(h.cfg.OpenAIAPIKey) == "" {
		return templateSteps, false, nil
	}
	if len(codes) == 0 || len(templateSteps) == 0 {
		return templateSteps, false, nil
	}

	type rewriteInput struct {
		Status      string   `json:"status"`
		IsReturning bool     `json:"is_returning"`
		StepCodes   []string `json:"step_codes"`
		BaseArabic  []string `json:"base_arabic"`
		Instruction string   `json:"instruction"`
	}
	payloadInput := rewriteInput{
		Status:      detail.Lead.Status,
		IsReturning: detail.Lead.IsReturning,
		StepCodes:   codes,
		BaseArabic:  templateSteps,
		Instruction: "أعد صياغة نفس الخطوات بالعربية المبسطة للمشرفين. لا تغيّر الترتيب أو العدد أو المعنى.",
	}
	inputJSON, _ := json.Marshal(payloadInput)

	requestBody := map[string]interface{}{
		"model": h.cfg.OpenAIModel,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You rewrite Arabic operations steps. Keep logic fixed. Return JSON only: {\"steps_ar\":[...]} with same number/order as input.",
			},
			{
				"role":    "user",
				"content": string(inputJSON),
			},
		},
		"temperature": 0.2,
	}

	rawBody, _ := json.Marshal(requestBody)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return templateSteps, false, err
	}
	req.Header.Set("Authorization", "Bearer "+h.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return templateSteps, false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return templateSteps, false, fmt.Errorf("openai status %d", resp.StatusCode)
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return templateSteps, false, err
	}
	if len(completion.Choices) == 0 {
		return templateSteps, false, fmt.Errorf("empty choices")
	}

	content := sanitizeJSONText(completion.Choices[0].Message.Content)
	var parsed struct {
		StepsAR []string `json:"steps_ar"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return templateSteps, false, err
	}
	if len(parsed.StepsAR) != len(templateSteps) {
		return templateSteps, false, fmt.Errorf("invalid steps count")
	}
	valid := make([]string, 0, len(parsed.StepsAR))
	for _, step := range parsed.StepsAR {
		step = strings.TrimSpace(step)
		if step == "" || len(step) > 120 {
			return templateSteps, false, fmt.Errorf("invalid step content")
		}
		valid = append(valid, step)
	}
	return valid, true, nil
}

func (h *PreEnrolmentHandler) buildSmartStepsForDetail(detail *models.LeadDetail, isFullyPaid bool, creditsRemaining int32, finalPriceValue, totalCoursePaid int32, lastOutcome string) ([]string, []string, string) {
	if detail != nil && detail.Lead != nil && detail.Lead.Status == "lead_created" {
		if sleepingLead, err := models.GetSleepingLeadByID(detail.Lead.ID); err == nil && sleepingLead != nil {
			return sleepingLeadSmartSteps(sleepingLead)
		}
	}

	codes := deterministicSmartStepCodes(detail, isFullyPaid, creditsRemaining, finalPriceValue, totalCoursePaid, lastOutcome)
	baseArabic := translateSmartStepCodesToArabic(codes)
	rewritten, usedAI, err := h.rewriteSmartStepsArabic(codes, baseArabic, detail)
	if err != nil {
		log.Printf("WARNING: Smart steps AI rewrite failed, fallback to template: %v", err)
		return codes, baseArabic, "template"
	}
	if usedAI {
		return codes, rewritten, "ai"
	}
	return codes, baseArabic, "template"
}
