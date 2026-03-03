package services

import (
	"time"

	"ticketing/config"
	"ticketing/models"
)

// StaffDashboardService provides data for the staff (department) dashboard.
type StaffDashboardService struct{}

func NewStaffDashboardService() *StaffDashboardService {
	return &StaffDashboardService{}
}

// StaffDashboardData holds all data for the staff dashboard.
type StaffDashboardData struct {
	// KPI
	WaitingCount     int     // pool
	ProgressCount    int     // my in-progress
	ClosedTodayCount int     // my closed today
	ClosedMonthCount int     // my closed this month
	AvgRating        float64 // avg rating on tickets I closed
	RatedCount       int     // how many of my closed have rating
	// Trend last N days: ambil (claimed), selesai (closed by me)
	TrendData []StaffTrendPoint
	// Monthly bar: tiket selesai per bulan (saya)
	MonthlyData []StaffMonthlyPoint
	// Donut: Di Pool, Saya Kerjakan, Selesai Milik Saya
	DonutData []StaffDonutPoint
	// Lists
	MyActiveTickets []*models.Ticket
	TicketPool      []*models.Ticket
	UnratedTickets  []*models.Ticket // my closed, no rating yet
	// Meta
	DepartmentName string
	TrendClosedPct int // % vs yesterday for "Selesai Hari Ini"
	TrendMonthPct  int // % vs last month for "Selesai Bulan Ini"
}

type StaffTrendPoint struct {
	Date    string `json:"d"`
	Ambil   int    `json:"ambil"`
	Selesai int    `json:"selesai"`
}

type StaffMonthlyPoint struct {
	B string `json:"b"` // month label
	V int    `json:"v"` // count
}

type StaffDonutPoint struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Color string `json:"color"`
}

const staffTrendDays = 10
const staffPoolRecent = 5
const staffUnratedLimit = 10

var (
	staffColorAmber = "#f59e0b"
	staffColorBlue  = "#3b82f6"
	staffColorGreen = "#10b981"
)

// GetStaffDashboardData returns full dashboard data for the given staff user.
func (s *StaffDashboardService) GetStaffDashboardData(userID uint, deptID uint) (*StaffDashboardData, error) {
	data := &StaffDashboardData{}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Department name
	var dept models.Department
	if config.DB.First(&dept, deptID).Error == nil {
		data.DepartmentName = dept.Name
	}

	// KPI: Pool count
	var n64 int64
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id IS NULL AND status = ? AND deleted_at IS NULL", models.StatusWaiting).Count(&n64)
	data.WaitingCount = int(n64)

	// KPI: My in-progress
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id = ? AND status = ?", userID, models.StatusInProgress).Count(&n64)
	data.ProgressCount = int(n64)

	// KPI: My closed today
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id = ? AND status = ? AND updated_at >= ?", userID, models.StatusClosed, todayStart).Count(&n64)
	data.ClosedTodayCount = int(n64)

	// KPI: My closed this month
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id = ? AND status = ? AND updated_at >= ?", userID, models.StatusClosed, monthStart).Count(&n64)
	data.ClosedMonthCount = int(n64)

	// Rating: avg from ticket_ratings for tickets where I was the assignee (closed by me)
	config.DB.Raw(`
		SELECT COALESCE(AVG(r.rating), 0) FROM ticket_ratings r
		INNER JOIN tickets t ON t.id = r.ticket_id AND t.assigned_to_id = ? AND t.status = 'CLOSED'
	`, userID).Scan(&data.AvgRating)
	config.DB.Raw(`
		SELECT COUNT(*) FROM ticket_ratings r
		INNER JOIN tickets t ON t.id = r.ticket_id AND t.assigned_to_id = ? AND t.deleted_at IS NULL
	`, userID).Scan(&n64)
	data.RatedCount = int(n64)

	// Trend data: last N days
	data.TrendData = s.getStaffTrendData(userID, staffTrendDays)

	// Monthly data: tickets I closed per month (this year)
	data.MonthlyData = s.getStaffMonthlyData(userID, now.Year())

	// Donut: Di Pool, Saya Kerjakan, Selesai Milik Saya
	var poolCount, myClosedTotal int64
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id IS NULL AND status = ? AND deleted_at IS NULL", models.StatusWaiting).Count(&poolCount)
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id = ? AND status = ?", userID, models.StatusInProgress).Count(&n64)
	myProgress := int(n64)
	config.DB.Model(&models.Ticket{}).Where("assigned_to_id = ? AND status = ?", userID, models.StatusClosed).Count(&myClosedTotal)
	data.DonutData = []StaffDonutPoint{
		{Name: "Di Pool", Value: int(poolCount), Color: staffColorAmber},
		{Name: "Saya Kerjakan", Value: myProgress, Color: staffColorBlue},
		{Name: "Selesai Milik Saya", Value: int(myClosedTotal), Color: staffColorGreen},
	}

	// My active tickets
	config.DB.Preload("Department").Preload("CreatedBy").
		Where("assigned_to_id = ? AND status != ?", userID, models.StatusClosed).
		Order("updated_at DESC").
		Find(&data.MyActiveTickets)

	// Pool
	config.DB.Preload("Department").Preload("CreatedBy").
		Where("assigned_to_id IS NULL AND status = ? AND deleted_at IS NULL", models.StatusWaiting).
		Order("created_at ASC").
		Find(&data.TicketPool)

	// Unrated: my closed tickets that have no rating
	config.DB.Preload("Department").
		Where("assigned_to_id = ? AND status = ? AND deleted_at IS NULL", userID, models.StatusClosed).
		Where("id NOT IN (SELECT ticket_id FROM ticket_ratings)").
		Order("updated_at DESC").Limit(staffUnratedLimit).
		Find(&data.UnratedTickets)

	// Trend %
	s.fillStaffTrendPct(data, userID)

	return data, nil
}

func (s *StaffDashboardService) getStaffTrendData(userID uint, days int) []StaffTrendPoint {
	now := time.Now()
	out := make([]StaffTrendPoint, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		dayStart := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		dayEnd := dayStart.Add(24 * time.Hour)

		var ambil int64
		config.DB.Model(&models.TicketAssignmentHistory{}).
			Where("staff_id = ? AND assigned_at >= ? AND assigned_at < ?", userID, dayStart, dayEnd).
			Count(&ambil)
		var selesai int64
		config.DB.Model(&models.Ticket{}).
			Where("assigned_to_id = ? AND status = ? AND updated_at >= ? AND updated_at < ?",
				userID, models.StatusClosed, dayStart, dayEnd).
			Count(&selesai)

		label := d.Format("2/1")
		out = append(out, StaffTrendPoint{Date: label, Ambil: int(ambil), Selesai: int(selesai)})
	}
	return out
}

func (s *StaffDashboardService) getStaffMonthlyData(userID uint, year int) []StaffMonthlyPoint {
	months := []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Ags", "Sep", "Okt", "Nov", "Des"}
	out := make([]StaffMonthlyPoint, 0, 12)
	for m := 1; m <= 12; m++ {
		monthStart := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		var monthEnd time.Time
		if m == 12 {
			monthEnd = time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
		} else {
			monthEnd = time.Date(year, time.Month(m+1), 1, 0, 0, 0, 0, time.UTC)
		}
		var n64 int64
		config.DB.Model(&models.Ticket{}).
			Where("assigned_to_id = ? AND status = ? AND updated_at >= ? AND updated_at < ?",
				userID, models.StatusClosed, monthStart, monthEnd).
			Count(&n64)
		out = append(out, StaffMonthlyPoint{B: months[m-1], V: int(n64)})
	}
	return out
}

func (s *StaffDashboardService) fillStaffTrendPct(data *StaffDashboardData, userID uint) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := yesterdayStart.Add(24 * time.Hour)
	var closedYesterday int64
	config.DB.Model(&models.Ticket{}).
		Where("assigned_to_id = ? AND status = ? AND updated_at >= ? AND updated_at < ?",
			userID, models.StatusClosed, yesterdayStart, yesterdayEnd).
		Count(&closedYesterday)
	if closedYesterday > 0 {
		data.TrendClosedPct = int(float64(data.ClosedTodayCount-int(closedYesterday)) / float64(closedYesterday) * 100)
	}

	lastMonthStart := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
	var closedLastMonth int64
	config.DB.Model(&models.Ticket{}).
		Where("assigned_to_id = ? AND status = ? AND updated_at >= ? AND updated_at < ?",
			userID, models.StatusClosed, lastMonthStart, monthStart).
		Count(&closedLastMonth)
	if closedLastMonth > 0 {
		data.TrendMonthPct = int(float64(data.ClosedMonthCount-int(closedLastMonth)) / float64(closedLastMonth) * 100)
	}
}
