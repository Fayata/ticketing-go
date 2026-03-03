package services

import (
	"fmt"
	"time"

	"ticketing/config"
	"ticketing/models"
)

// AdminDashboardService provides data for the admin dashboard.
type AdminDashboardService struct{}

func NewAdminDashboardService() *AdminDashboardService {
	return &AdminDashboardService{}
}

// AdminDashboardData holds all data for the admin dashboard page.
type AdminDashboardData struct {
	// KPI
	WaitingCount      int
	InProgressCount   int
	ClosedTodayCount  int
	AvgRating         float64
	RatedCount        int
	TotalTicketsMonth int
	TotalUsersActive  int
	StaffActiveCount  int
	UnratedCount      int
	// Trend (last N days): Tiket Baru & Selesai per hari
	TrendData []AdminTrendPoint
	// Status pie: Menunggu, In Progress, Closed
	StatusData []AdminStatusPoint
	// Tiket per departemen (horizontal bar)
	DeptData []AdminDeptPoint
	// Tiket per prioritas
	PriorityData []AdminPriorityPoint
	// Distribusi rating 1-5
	RatingData []AdminRatingPoint
	// Tiket terbaru (table)
	RecentTickets []*models.Ticket
	// Menunggu terlama (pool), dengan jumlah hari
	WaitingLongest []AdminWaitingItem
	// Staff per departemen + tiket aktif per dept
	StaffData []AdminStaffDeptPoint
	// Optional trend % for KPI cards (vs previous period)
	TrendWaitingPct      int
	TrendProgressPct     int
	TrendClosedTodayPct  int
	TrendAvgRatingPct    int
}

type AdminTrendPoint struct {
	Date    string `json:"d"`
	Baru    int    `json:"baru"`
	Selesai int    `json:"selesai"`
}

type AdminStatusPoint struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Color string `json:"color"`
}

type AdminDeptPoint struct {
	Dept  string `json:"dept"`
	Tiket int    `json:"tiket"`
}

type AdminPriorityPoint struct {
	Label string `json:"label"`
	Count int    `json:"count"`
	Color string `json:"color"`
}

type AdminRatingPoint struct {
	Star  string `json:"star"`
	Count int    `json:"count"`
	Pct   int    `json:"pct"` // 0-100 for bar width
}

type AdminStaffDeptPoint struct {
	Dept   string `json:"dept"`
	Staff  int    `json:"staff"`
	Aktif  int    `json:"aktif"`
}

// AdminWaitingItem dipakai untuk list "Menunggu Terlama" (tiket + hari menunggu).
type AdminWaitingItem struct {
	Ticket *models.Ticket
	Days   int
}

const (
	trendDays     = 10
	recentLimit   = 6
	waitingLimit  = 5
	statusAmber   = "#f59e0b"
	statusBlue    = "#3b82f6"
	statusGreen   = "#10b981"
	priorityRed   = "#ef4444"
	priorityAmber = "#f59e0b"
	priorityGreen = "#10b981"
)

// GetAdminDashboardData returns full data for the admin dashboard.
func (s *AdminDashboardService) GetAdminDashboardData() (*AdminDashboardData, error) {
	data := &AdminDashboardData{}

	// ─── KPI: counts ───
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var n64 int64
	config.DB.Model(&models.Ticket{}).Where("status = ? AND assigned_to_id IS NULL", models.StatusWaiting).Count(&n64)
	data.WaitingCount = int(n64)
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusInProgress).Count(&n64)
	data.InProgressCount = int(n64)
	config.DB.Model(&models.Ticket{}).Where("status = ? AND updated_at >= ?", models.StatusClosed, todayStart).Count(&n64)
	data.ClosedTodayCount = int(n64)
	config.DB.Model(&models.Ticket{}).Where("created_at >= ?", monthStart).Count(&n64)
	data.TotalTicketsMonth = int(n64)
	config.DB.Model(&models.User{}).Where("is_active = ?", true).Count(&n64)
	data.TotalUsersActive = int(n64)
	config.DB.Model(&models.User{}).Where("is_staff = ? AND is_active = ?", true, true).Count(&n64)
	data.StaffActiveCount = int(n64)

	var avg float64
	config.DB.Model(&models.TicketRating{}).Select("COALESCE(AVG(rating), 0)").Scan(&avg)
	data.AvgRating = avg
	config.DB.Model(&models.TicketRating{}).Count(&n64)
	data.RatedCount = int(n64)

	// Closed tapi belum di-rate
	config.DB.Raw(`
		SELECT COUNT(*) FROM tickets t
		WHERE t.status = ? AND t.deleted_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM ticket_ratings r WHERE r.ticket_id = t.id)
	`, models.StatusClosed).Scan(&n64)
	data.UnratedCount = int(n64)

	// ─── Trend: last N days ───
	data.TrendData = s.getTrendData(trendDays)

	// ─── Status pie ───
	var w, p, c int64
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusWaiting).Count(&w)
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusInProgress).Count(&p)
	config.DB.Model(&models.Ticket{}).Where("status = ?", models.StatusClosed).Count(&c)
	data.StatusData = []AdminStatusPoint{
		{Name: "Menunggu", Value: int(w), Color: statusAmber},
		{Name: "In Progress", Value: int(p), Color: statusBlue},
		{Name: "Closed", Value: int(c), Color: statusGreen},
	}

	// ─── Per departemen ───
	var depts []models.Department
	config.DB.Order("name").Find(&depts)
	data.DeptData = make([]AdminDeptPoint, 0, len(depts)+1)
	for _, d := range depts {
		var n int64
		config.DB.Model(&models.Ticket{}).Where("department_id = ?", d.ID).Count(&n)
		data.DeptData = append(data.DeptData, AdminDeptPoint{Dept: d.Name, Tiket: int(n)})
	}
	var noDept int64
	config.DB.Model(&models.Ticket{}).Where("department_id IS NULL").Count(&noDept)
	if noDept > 0 {
		data.DeptData = append(data.DeptData, AdminDeptPoint{Dept: "Tanpa Dept", Tiket: int(noDept)})
	}

	// ─── Per prioritas ───
	var high, med, low int64
	config.DB.Model(&models.Ticket{}).Where("priority = ?", models.PriorityHigh).Count(&high)
	config.DB.Model(&models.Ticket{}).Where("priority = ?", models.PriorityMedium).Count(&med)
	config.DB.Model(&models.Ticket{}).Where("priority = ?", models.PriorityLow).Count(&low)
	data.PriorityData = []AdminPriorityPoint{
		{Label: "High", Count: int(high), Color: priorityRed},
		{Label: "Medium", Count: int(med), Color: priorityAmber},
		{Label: "Low", Count: int(low), Color: priorityGreen},
	}

	// ─── Distribusi rating 1-5 ───
	data.RatingData = s.getRatingDistribution()

	// ─── Tiket terbaru ───
	config.DB.Preload("Department").Where("deleted_at IS NULL").
		Order("created_at DESC").Limit(recentLimit).Find(&data.RecentTickets)

	// ─── Menunggu terlama (pool, order by created_at asc) ───
	var waitingTickets []*models.Ticket
	config.DB.Preload("Department").Where("status = ? AND assigned_to_id IS NULL AND deleted_at IS NULL",
		models.StatusWaiting).Order("created_at ASC").Limit(waitingLimit).Find(&waitingTickets)
	data.WaitingLongest = make([]AdminWaitingItem, 0, len(waitingTickets))
	for _, t := range waitingTickets {
		days := int(now.Sub(t.CreatedAt).Hours() / 24)
		if days < 0 {
			days = 0
		}
		data.WaitingLongest = append(data.WaitingLongest, AdminWaitingItem{Ticket: t, Days: days})
	}

	// ─── Staff per departemen + tiket aktif per dept ───
	data.StaffData = s.getStaffDeptData()

	// Trend % (sederhana: bandingkan dengan periode sebelumnya)
	s.fillTrendPct(data)

	return data, nil
}

func (s *AdminDashboardService) getTrendData(days int) []AdminTrendPoint {
	now := time.Now()
	out := make([]AdminTrendPoint, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		dayStart := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		dayEnd := dayStart.Add(24 * time.Hour)

		var baru int64
		config.DB.Model(&models.Ticket{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&baru)
		var selesai int64
		config.DB.Model(&models.Ticket{}).Where("status = ? AND updated_at >= ? AND updated_at < ?",
			models.StatusClosed, dayStart, dayEnd).Count(&selesai)

		label := d.Format("2/1")
		out = append(out, AdminTrendPoint{Date: label, Baru: int(baru), Selesai: int(selesai)})
	}
	return out
}

func (s *AdminDashboardService) getRatingDistribution() []AdminRatingPoint {
	out := make([]AdminRatingPoint, 5)
	var total int64
	for i := 1; i <= 5; i++ {
		var n int64
		config.DB.Model(&models.TicketRating{}).Where("rating = ?", i).Count(&n)
		total += n
		out[5-i] = AdminRatingPoint{Star: fmt.Sprintf("%d ★", i), Count: int(n)}
	}
	if total > 0 {
		for i := range out {
			out[i].Pct = int(float64(out[i].Count) / float64(total) * 100)
			if out[i].Pct > 100 {
				out[i].Pct = 100
			}
		}
	}
	return out
}

func (s *AdminDashboardService) getStaffDeptData() []AdminStaffDeptPoint {
	var depts []models.Department
	config.DB.Order("name").Find(&depts)
	out := make([]AdminStaffDeptPoint, 0, len(depts))
	for _, d := range depts {
		var staff int64
		config.DB.Model(&models.User{}).Where("department_id = ? AND is_staff = ? AND is_active = ?", d.ID, true, true).Count(&staff)
		var aktif int64
		config.DB.Model(&models.Ticket{}).Where("department_id = ? AND status = ?", d.ID, models.StatusInProgress).Count(&aktif)
		out = append(out, AdminStaffDeptPoint{Dept: d.Name, Staff: int(staff), Aktif: int(aktif)})
	}
	return out
}

func (s *AdminDashboardService) fillTrendPct(data *AdminDashboardData) {
	// Sederhana: bandingkan hari ini vs kemarin untuk closed today; waiting vs 7d ago, dll.
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := yesterdayStart.Add(24 * time.Hour)
	var closedYesterday int64
	config.DB.Model(&models.Ticket{}).Where("status = ? AND updated_at >= ? AND updated_at < ?",
		models.StatusClosed, yesterdayStart, yesterdayEnd).Count(&closedYesterday)
	if closedYesterday > 0 {
		data.TrendClosedTodayPct = int(float64(data.ClosedTodayCount-int(closedYesterday)) / float64(closedYesterday) * 100)
	}
	// Lainnya bisa diisi 0 atau hitung serupa
}
