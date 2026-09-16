package services

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/internal/logging"
	"ticketing/models"
)

// MonthlyReportFilter menampung filter bulan, tahun, dan perusahaan.
type MonthlyReportFilter struct {
	Month     int   // 1 - 12
	Year      int   // misal 2026
	CompanyID *uint // opsional filter ke 1 PT
}

// MonthOption opsi untuk dropdown pilihan bulan di antarmuka.
type MonthOption struct {
	Month      int
	Year       int
	Label      string // misal: "September 2026"
	IsSelected bool
}

// DepartmentReportItem berisi rincian kinerja satu departemen di bawah satu perusahaan.
type DepartmentReportItem struct {
	DepartmentID           uint
	DepartmentName         string
	TotalTickets           int
	OpenTickets            int // WAITING + IN_PROGRESS
	ClosedTickets          int // CLOSED
	FirstResponseBreached  int // Respon pertama terlambat
	ResolutionBreached     int // Penyelesaian terlambat melebihi deadline
	SLAMetCount            int // Tiket yang sukses memenuhi SLA
	SLAMetRate             float64 // Persentase kepatuhan SLA (0 - 100%)
	AvgRating              float64 // Rata-rata bintang (1.0 - 5.0)
	RatedCount             int // Jumlah tiket yang dinilai
}

// CompanyReportItem berisi rekapitulasi satu perusahaan beserta sub-departemennya.
type CompanyReportItem struct {
	CompanyID              uint
	CompanyName            string
	CompanyCode            string
	Departments            []DepartmentReportItem
	
	// Subtotal per perusahaan
	TotalTickets           int
	OpenTickets            int
	ClosedTickets          int
	FirstResponseBreached  int
	ResolutionBreached     int
	SLAMetCount            int
	SLAMetRate             float64
	AvgRating              float64
	RatedCount             int
}

// GrandTotalSummary berisi akumulasi total seluruh perusahaan pada bulan tersebut.
type GrandTotalSummary struct {
	TotalTickets           int
	OpenTickets            int
	ClosedTickets          int
	FirstResponseBreached  int
	ResolutionBreached     int
	OverallSLAMetRate      float64
	OverallAvgRating       float64
	TotalRatedCount        int
}

// MonthlyReportData payload lengkap yang dikirimkan ke template HTML.
type MonthlyReportData struct {
	Filter                 MonthlyReportFilter
	MonthName              string
	Year                   int
	PeriodLabel            string // misal: "September 2026"
	MonthOptions           []MonthOption
	CompanyList            []CompanyReportItem
	GrandTotal             GrandTotalSummary
	AllCompanies           []models.Company // Untuk dropdown filter PT
	SelectedCompanyID      uint
	SelectedCompanyName    string // misal: "PT Utama (DEFAULT)" atau "Semua Perusahaan"
	GeneratedAtFormatted   string
}

// StaffReportItem berisi rincian kinerja satu staff di bawah satu departemen.
type StaffReportItem struct {
	StaffID               uint
	StaffName             string
	Username              string
	TotalTickets          int
	OpenTickets           int
	ClosedTickets         int
	FirstResponseBreached int
	ResolutionBreached    int
	SLAMetCount           int
	SLAMetRate            float64
	AvgRating             float64
	RatedCount            int
}

// DepartmentDetailReportData payload lengkap untuk laporan kinerja staff per departemen.
type DepartmentDetailReportData struct {
	DepartmentID         uint
	DepartmentName       string
	CompanyID            uint
	CompanyName          string
	CompanyCode          string
	PeriodLabel          string
	Month                int
	Year                 int
	Summary              DepartmentReportItem
	StaffList            []StaffReportItem
	UnassignedItem       *StaffReportItem
	GeneratedAtFormatted string
	AutoPrint            bool
}

// AdminReportService mengelola pembuatan laporan kinerja bulanan.
type AdminReportService struct{}

func NewAdminReportService() *AdminReportService {
	return &AdminReportService{}
}

// indonesianMonths nama bulan dalam bahasa Indonesia.
var indonesianMonths = []string{
	"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// GetMonthName mengembalikan nama bulan Indonesia dari angka 1-12.
func GetMonthName(m int) string {
	if m >= 1 && m <= 12 {
		return indonesianMonths[m]
	}
	return ""
}

// GetMonthOptions menghasilkan daftar pilihan bulan (12 bulan terakhir + bulan berjalan).
func (s *AdminReportService) GetMonthOptions(selectedMonth, selectedYear int) []MonthOption {
	now := time.Now()
	var options []MonthOption

	// Tampilkan 12 bulan ke belakang mulai dari bulan sekarang
	for i := 0; i < 12; i++ {
		d := now.AddDate(0, -i, 0)
		m := int(d.Month())
		y := d.Year()
		label := fmt.Sprintf("%s %d", GetMonthName(m), y)
		isSelected := (m == selectedMonth && y == selectedYear)
		options = append(options, MonthOption{
			Month:      m,
			Year:       y,
			Label:      label,
			IsSelected: isSelected,
		})
	}
	return options
}

// GetMonthlyCompanyReport mengagregasi data kinerja tiket per PT dan per departemen untuk bulan terpilih.
func (s *AdminReportService) GetMonthlyCompanyReport(filter MonthlyReportFilter) (*MonthlyReportData, error) {
	now := time.Now()
	wibZone := time.FixedZone("WIB", 7*3600)

	// Default ke bulan dan tahun sekarang jika tidak dispesifikasikan
	if filter.Month < 1 || filter.Month > 12 {
		filter.Month = int(now.Month())
	}
	if filter.Year < 2020 {
		filter.Year = now.Year()
	}

	var selectedCID uint
	if filter.CompanyID != nil {
		selectedCID = *filter.CompanyID
	}

	selectedCompName := "Semua Perusahaan"
	if selectedCID > 0 {
		selectedCompName = fmt.Sprintf("Perusahaan #%d", selectedCID)
	}

	logging.AdminReports.Info("GetMonthlyCompanyReport started",
		"filter_month", filter.Month,
		"filter_year", filter.Year,
		"filter_company_id", selectedCID,
	)

	report := &MonthlyReportData{
		Filter:               filter,
		MonthName:            GetMonthName(filter.Month),
		Year:                 filter.Year,
		PeriodLabel:          fmt.Sprintf("%s %d", GetMonthName(filter.Month), filter.Year),
		MonthOptions:         s.GetMonthOptions(filter.Month, filter.Year),
		SelectedCompanyID:    selectedCID,
		SelectedCompanyName:  selectedCompName,
		GeneratedAtFormatted: now.In(wibZone).Format("02 Jan 2006, 15:04") + " WIB",
	}

	if config.DB == nil {
		logging.AdminReports.Warn("Database instance is nil, returning empty report")
		return report, nil
	}

	// 1. Ambil daftar semua perusahaan aktif untuk opsi filter dan master data
	var allCompanies []models.Company
	config.DB.Order("name ASC").Find(&allCompanies)
	report.AllCompanies = allCompanies

	if selectedCID > 0 {
		for _, c := range allCompanies {
			if c.ID == selectedCID {
				if c.Code != "" {
					report.SelectedCompanyName = fmt.Sprintf("%s (%s)", c.Name, c.Code)
				} else {
					report.SelectedCompanyName = c.Name
				}
				break
			}
		}
	}

	// 2. Ambil master departemen
	var allDepts []models.Department
	config.DB.Preload("Company").Order("name ASC").Find(&allDepts)

	// DB Diagnostics logging
	var totalTicketsAll int64
	config.DB.Model(&models.Ticket{}).Count(&totalTicketsAll)
	logging.AdminReports.Info("Database diagnostics",
		"total_tickets_in_db", totalTicketsAll,
		"companies_master_count", len(allCompanies),
		"departments_master_count", len(allDepts),
	)

	// Log sampel tiket terbaru di DB (maks 5) untuk memudahkan verifikasi
	var sampleTickets []models.Ticket
	config.DB.Order("id desc").Limit(5).Find(&sampleTickets)
	for _, st := range sampleTickets {
		var cidVal uint
		if st.CompanyID != nil {
			cidVal = *st.CompanyID
		}
		var didVal uint
		if st.DepartmentID != nil {
			didVal = *st.DepartmentID
		}
		logging.AdminReports.Debug("DB sample ticket",
			"id", st.ID,
			"ticket_number", st.GetTicketNumber(),
			"title", st.Title,
			"status", st.Status,
			"created_at", st.CreatedAt.Format(time.RFC3339),
			"department_id", didVal,
			"company_id", cidVal,
		)
	}

	// Filter companies jika user memilih 1 PT spesifik
	targetCompanies := make([]models.Company, 0)
	for _, comp := range allCompanies {
		if filter.CompanyID != nil && *filter.CompanyID > 0 && comp.ID != *filter.CompanyID {
			continue
		}
		targetCompanies = append(targetCompanies, comp)
	}

	type deptKey struct {
		CompanyID    uint
		DepartmentID uint
	}

	type deptStats struct {
		companyID             uint
		companyName           string
		companyCode           string
		departmentID          uint
		departmentName        string
		totalTickets          int
		openTickets           int
		closedTickets         int
		firstResponseBreached int
		resolutionBreached    int
		slaMetCount           int
		totalRatingScore      int
		ratedCount            int
	}

	deptStatsMap := make(map[deptKey]*deptStats)
	companyOrder := make([]uint, 0)
	companySeen := make(map[uint]bool)
	companyMeta := make(map[uint]struct{ name, code string })

	// Master lookup map untuk metadata perusahaan
	for _, comp := range allCompanies {
		companyMeta[comp.ID] = struct{ name, code string }{name: comp.Name, code: comp.Code}
	}
	companyMeta[0] = struct{ name, code string }{name: "Umum / Tanpa Perusahaan", code: "DEFAULT"}

	// 1) Daftarkan perusahaan terpilih beserta departemennya
	for _, comp := range targetCompanies {
		cID := comp.ID
		if !companySeen[cID] {
			companySeen[cID] = true
			companyOrder = append(companyOrder, cID)
		}

		for _, dept := range allDepts {
			if dept.CompanyID != nil && *dept.CompanyID == comp.ID {
				key := deptKey{CompanyID: comp.ID, DepartmentID: dept.ID}
				if _, exists := deptStatsMap[key]; !exists {
					deptStatsMap[key] = &deptStats{
						companyID:      comp.ID,
						companyName:    comp.Name,
						companyCode:    comp.Code,
						departmentID:   dept.ID,
						departmentName: dept.Name,
					}
				}
			}
		}
	}

	// 2) Daftarkan departemen yang belum terdaftar (misal tanpa PT atau PT tidak ada di tabel perusahaan)
	for _, dept := range allDepts {
		var deptCID uint
		if dept.CompanyID != nil && *dept.CompanyID > 0 {
			deptCID = *dept.CompanyID
		}

		if filter.CompanyID != nil && *filter.CompanyID > 0 && deptCID != *filter.CompanyID {
			continue
		}

		cName := "Umum / Tanpa Perusahaan"
		cCode := "DEFAULT"
		if deptCID > 0 {
			if meta, ok := companyMeta[deptCID]; ok {
				cName = meta.name
				cCode = meta.code
			} else if dept.Company != nil && dept.Company.Name != "" {
				cName = dept.Company.Name
				cCode = dept.Company.Code
				companyMeta[deptCID] = struct{ name, code string }{name: cName, code: cCode}
			} else {
				cName = fmt.Sprintf("Perusahaan #%d", deptCID)
				cCode = "CORP"
				companyMeta[deptCID] = struct{ name, code string }{name: cName, code: cCode}
			}
		}

		if !companySeen[deptCID] {
			companySeen[deptCID] = true
			companyOrder = append(companyOrder, deptCID)
		}

		key := deptKey{CompanyID: deptCID, DepartmentID: dept.ID}
		if _, exists := deptStatsMap[key]; !exists {
			deptStatsMap[key] = &deptStats{
				companyID:      deptCID,
				companyName:    cName,
				companyCode:    cCode,
				departmentID:   dept.ID,
				departmentName: dept.Name,
			}
		}
	}

	// Fallback jika tidak ada perusahaan/departemen sama sekali, daftarkan grup 0 agar tabel tidak hilang
	if len(companyOrder) == 0 && (filter.CompanyID == nil || *filter.CompanyID == 0) {
		companySeen[0] = true
		companyOrder = append(companyOrder, 0)
	}

	// 3. Query semua tiket pada rentang bulan ini (kompatibel lintas timezone)
	loc := now.Location()
	startOfMonth := time.Date(filter.Year, time.Month(filter.Month), 1, 0, 0, 0, 0, loc)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	startOfMonthUTC := time.Date(filter.Year, time.Month(filter.Month), 1, 0, 0, 0, 0, time.UTC)
	endOfMonthUTC := startOfMonthUTC.AddDate(0, 1, 0)

	logging.AdminReports.Info("Querying monthly tickets for report",
		"month", filter.Month,
		"year", filter.Year,
		"start_local", startOfMonth.Format(time.RFC3339),
		"end_local", endOfMonth.Format(time.RFC3339),
		"start_utc", startOfMonthUTC.Format(time.RFC3339),
		"end_utc", endOfMonthUTC.Format(time.RFC3339),
	)

	query := config.DB.Model(&models.Ticket{}).
		Preload("Company").
		Preload("Department").
		Preload("Department.Company").
		Where("(created_at >= ? AND created_at < ?) OR (created_at >= ? AND created_at < ?) OR (EXTRACT(MONTH FROM created_at) = ? AND EXTRACT(YEAR FROM created_at) = ?)",
			startOfMonth, endOfMonth, startOfMonthUTC, endOfMonthUTC, filter.Month, filter.Year)

	// Filter per PT jika dipilih: sertakan tiket dengan company_id bersangkutan ATAU departemennya berafiliasi dengan PT tsb
	if filter.CompanyID != nil && *filter.CompanyID > 0 {
		query = query.Where("(tickets.company_id = ? OR tickets.department_id IN (SELECT id FROM departments WHERE company_id = ?))", *filter.CompanyID, *filter.CompanyID)
	}

	var tickets []models.Ticket
	if err := query.Order("created_at ASC").Find(&tickets).Error; err != nil {
		logging.AdminReports.Error("Failed to execute tickets query for monthly report", "error", err.Error())
		return nil, err
	}

	logging.AdminReports.Info("Monthly tickets query finished",
		"tickets_matched_count", len(tickets),
		"filter_month", filter.Month,
		"filter_year", filter.Year,
	)

	// Ambil rating untuk tiket yang terpilih
	var ticketIDs []uint
	for _, t := range tickets {
		ticketIDs = append(ticketIDs, t.ID)
	}
	ratingsMap := make(map[uint]int)
	if len(ticketIDs) > 0 {
		var ratings []models.TicketRating
		config.DB.Where("ticket_id IN ?", ticketIDs).Find(&ratings)
		for _, r := range ratings {
			ratingsMap[r.TicketID] = r.Rating
		}
	}

	// 4. Iterasi tiket untuk agregasi data
	for _, t := range tickets {
		cID := uint(0)
		cName := "Umum / Tanpa Perusahaan"
		cCode := "DEFAULT"

		// Resolusi CompanyID dari tiket atau departemen terkait
		if t.CompanyID != nil && *t.CompanyID > 0 {
			cID = *t.CompanyID
			if t.Company != nil && t.Company.Name != "" {
				cName = t.Company.Name
				cCode = t.Company.Code
			} else if meta, ok := companyMeta[cID]; ok {
				cName = meta.name
				cCode = meta.code
			}
		} else if t.Department != nil && t.Department.CompanyID != nil && *t.Department.CompanyID > 0 {
			cID = *t.Department.CompanyID
			if t.Department.Company != nil && t.Department.Company.Name != "" {
				cName = t.Department.Company.Name
				cCode = t.Department.Company.Code
			} else if meta, ok := companyMeta[cID]; ok {
				cName = meta.name
				cCode = meta.code
			}
		} else if t.DepartmentID != nil && *t.DepartmentID > 0 {
			for _, d := range allDepts {
				if d.ID == *t.DepartmentID && d.CompanyID != nil && *d.CompanyID > 0 {
					cID = *d.CompanyID
					if meta, ok := companyMeta[cID]; ok {
						cName = meta.name
						cCode = meta.code
					}
					break
				}
			}
		}

		if filter.CompanyID != nil && *filter.CompanyID > 0 && cID != *filter.CompanyID {
			continue
		}

		dID := uint(0)
		dName := "Umum / Tanpa Departemen"
		if t.DepartmentID != nil && *t.DepartmentID > 0 {
			dID = *t.DepartmentID
			if t.Department != nil && t.Department.Name != "" {
				dName = t.Department.Name
			} else {
				for _, d := range allDepts {
					if d.ID == *t.DepartmentID {
						dName = d.Name
						break
					}
				}
			}
		}

		logging.AdminReports.Debug("Ticket mapped in monthly report",
			"ticket_id", t.ID,
			"ticket_number", t.GetTicketNumber(),
			"status", t.Status,
			"created_at", t.CreatedAt.Format(time.RFC3339),
			"resolved_company_id", cID,
			"resolved_company_name", cName,
			"resolved_department_id", dID,
			"resolved_department_name", dName,
		)

		if !companySeen[cID] {
			companySeen[cID] = true
			companyOrder = append(companyOrder, cID)
			companyMeta[cID] = struct{ name, code string }{name: cName, code: cCode}
		}

		key := deptKey{CompanyID: cID, DepartmentID: dID}
		ds, exists := deptStatsMap[key]
		if !exists {
			ds = &deptStats{
				companyID:      cID,
				companyName:    cName,
				companyCode:    cCode,
				departmentID:   dID,
				departmentName: dName,
			}
			deptStatsMap[key] = ds
		}

		ds.totalTickets++
		if t.Status == models.StatusClosed {
			ds.closedTickets++
		} else {
			ds.openTickets++
		}

		// Evaluasi First Response SLA
		isFRBreached := false
		if t.FirstResponseMet != nil && !*t.FirstResponseMet {
			isFRBreached = true
		} else if t.FirstResponseMet == nil && t.FirstResponseDeadline != nil && now.After(*t.FirstResponseDeadline) {
			isFRBreached = true
		}
		if isFRBreached {
			ds.firstResponseBreached++
		}

		// Evaluasi Resolution SLA
		isResBreached := false
		if t.Status == models.StatusClosed {
			if t.ResolutionDeadline != nil && t.UpdatedAt.After(*t.ResolutionDeadline) {
				isResBreached = true
			} else if t.EstimatedResolutionAt != nil && t.UpdatedAt.After(*t.EstimatedResolutionAt) {
				isResBreached = true
			}
		} else {
			if t.ResolutionDeadline != nil && now.After(*t.ResolutionDeadline) {
				isResBreached = true
			} else if t.EstimatedResolutionAt != nil && now.After(*t.EstimatedResolutionAt) {
				isResBreached = true
			}
		}
		if isResBreached {
			ds.resolutionBreached++
		}

		// Kepatuhan SLA: Tidak melanggar respon DAN tidak melanggar resolusi
		if !isFRBreached && !isResBreached {
			ds.slaMetCount++
		}

		// Rating
		if r, ok := ratingsMap[t.ID]; ok && r >= 1 && r <= 5 {
			ds.totalRatingScore += r
			ds.ratedCount++
		}
	}

	// Bangun CompanyReportItem
	var companyList []CompanyReportItem
	var grandTotal GrandTotalSummary
	var grandTotalRatingScore int
	var grandSLAMetTotal int

	for _, cID := range companyOrder {
		meta := companyMeta[cID]
		compItem := CompanyReportItem{
			CompanyID:   cID,
			CompanyName: meta.name,
			CompanyCode: meta.code,
		}

		var deptItems []DepartmentReportItem
		var compRatingScore int

		// Kumpulkan departemen di bawah perusahaan ini
		for key, ds := range deptStatsMap {
			if key.CompanyID != cID {
				continue
			}

			item := DepartmentReportItem{
				DepartmentID:          ds.departmentID,
				DepartmentName:        ds.departmentName,
				TotalTickets:          ds.totalTickets,
				OpenTickets:           ds.openTickets,
				ClosedTickets:         ds.closedTickets,
				FirstResponseBreached: ds.firstResponseBreached,
				ResolutionBreached:    ds.resolutionBreached,
				SLAMetCount:           ds.slaMetCount,
				RatedCount:            ds.ratedCount,
			}

			if ds.totalTickets > 0 {
				item.SLAMetRate = math.Round((float64(ds.slaMetCount)/float64(ds.totalTickets)*100)*10) / 10
			} else {
				item.SLAMetRate = 0.0
			}

			if ds.ratedCount > 0 {
				item.AvgRating = math.Round((float64(ds.totalRatingScore)/float64(ds.ratedCount))*10) / 10
			}

			deptItems = append(deptItems, item)

			// Akumulasi subtotal PT
			compItem.TotalTickets += ds.totalTickets
			compItem.OpenTickets += ds.openTickets
			compItem.ClosedTickets += ds.closedTickets
			compItem.FirstResponseBreached += ds.firstResponseBreached
			compItem.ResolutionBreached += ds.resolutionBreached
			compItem.SLAMetCount += ds.slaMetCount
			compItem.RatedCount += ds.ratedCount
			compRatingScore += ds.totalRatingScore
		}

		// Urutkan departemen berdasarkan total tiket terbanyak
		sort.Slice(deptItems, func(i, j int) bool {
			return deptItems[i].TotalTickets > deptItems[j].TotalTickets
		})
		compItem.Departments = deptItems

		// Hitung SLA rate & Rating subtotal PT
		if compItem.TotalTickets > 0 {
			compItem.SLAMetRate = math.Round((float64(compItem.SLAMetCount)/float64(compItem.TotalTickets)*100)*10) / 10
		} else {
			compItem.SLAMetRate = 0.0
		}
		if compItem.RatedCount > 0 {
			compItem.AvgRating = math.Round((float64(compRatingScore)/float64(compItem.RatedCount))*10) / 10
		}

		companyList = append(companyList, compItem)

		// Akumulasi Grand Total
		grandTotal.TotalTickets += compItem.TotalTickets
		grandTotal.OpenTickets += compItem.OpenTickets
		grandTotal.ClosedTickets += compItem.ClosedTickets
		grandTotal.FirstResponseBreached += compItem.FirstResponseBreached
		grandTotal.ResolutionBreached += compItem.ResolutionBreached
		grandTotal.TotalRatedCount += compItem.RatedCount
		grandTotalRatingScore += compRatingScore
		grandSLAMetTotal += compItem.SLAMetCount
	}

	// Hitung Grand Total SLA & Rating
	if grandTotal.TotalTickets > 0 {
		grandTotal.OverallSLAMetRate = math.Round((float64(grandSLAMetTotal)/float64(grandTotal.TotalTickets)*100)*10) / 10
	} else {
		grandTotal.OverallSLAMetRate = 0.0
	}
	if grandTotal.TotalRatedCount > 0 {
		grandTotal.OverallAvgRating = math.Round((float64(grandTotalRatingScore)/float64(grandTotal.TotalRatedCount))*10) / 10
	}

	report.CompanyList = companyList
	report.GrandTotal = grandTotal

	logging.AdminReports.Info("GetMonthlyCompanyReport completed",
		"filter_month", filter.Month,
		"filter_year", filter.Year,
		"companies_in_report", len(companyList),
		"grand_total_tickets", grandTotal.TotalTickets,
		"open_tickets", grandTotal.OpenTickets,
		"closed_tickets", grandTotal.ClosedTickets,
		"first_response_breached", grandTotal.FirstResponseBreached,
		"resolution_breached", grandTotal.ResolutionBreached,
		"overall_sla_met_rate", grandTotal.OverallSLAMetRate,
		"total_rated_count", grandTotal.TotalRatedCount,
		"overall_avg_rating", grandTotal.OverallAvgRating,
	)

	return report, nil
}

// GetDepartmentStaffReport mengagregasi data kinerja seluruh staff di satu departemen pada bulan terpilih.
func (s *AdminReportService) GetDepartmentStaffReport(deptID uint, month int, year int) (*DepartmentDetailReportData, error) {
	now := time.Now()
	wibZone := time.FixedZone("WIB", 7*3600)

	if month < 1 || month > 12 {
		month = int(now.Month())
	}
	if year < 2020 {
		year = now.Year()
	}

	startMonthWIB := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, wibZone)
	endMonthWIB := startMonthWIB.AddDate(0, 1, 0)

	startMonthUTC := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endMonthUTC := startMonthUTC.AddDate(0, 1, 0)

	logging.AdminReports.Info("GetDepartmentStaffReport started",
		"department_id", deptID,
		"month", month,
		"year", year,
	)

	report := &DepartmentDetailReportData{
		DepartmentID:         deptID,
		DepartmentName:       fmt.Sprintf("Departemen #%d", deptID),
		PeriodLabel:          fmt.Sprintf("%s %d", GetMonthName(month), year),
		Month:                month,
		Year:                 year,
		GeneratedAtFormatted: now.In(wibZone).Format("02 Jan 2006, 15:04") + " WIB",
	}

	if config.DB == nil {
		logging.AdminReports.Warn("Database instance is nil, returning empty department report")
		return report, nil
	}

	var dept models.Department
	if err := config.DB.Preload("Company").First(&dept, deptID).Error; err != nil {
		logging.AdminReports.Error("Department not found", "department_id", deptID, "error", err.Error())
		return nil, fmt.Errorf("departemen dengan ID %d tidak ditemukan: %w", deptID, err)
	}

	report.DepartmentName = dept.Name
	if dept.Company != nil {
		report.CompanyID = dept.Company.ID
		report.CompanyName = dept.Company.Name
		report.CompanyCode = dept.Company.Code
	} else if dept.CompanyID != nil {
		report.CompanyID = *dept.CompanyID
		var comp models.Company
		if err := config.DB.First(&comp, *dept.CompanyID).Error; err == nil {
			report.CompanyName = comp.Name
			report.CompanyCode = comp.Code
		} else {
			report.CompanyName = fmt.Sprintf("Perusahaan #%d", *dept.CompanyID)
		}
	} else {
		report.CompanyName = "Umum / Tanpa Perusahaan"
	}

	// 1. Ambil seluruh staff yang terdaftar di departemen ini
	var staffUsers []models.User
	config.DB.Where("department_id = ? AND is_staff = ?", deptID, true).
		Order("first_name ASC, username ASC").
		Find(&staffUsers)

	staffMap := make(map[uint]*StaffReportItem)
	var staffList []*StaffReportItem
	staffRatingScores := make(map[*StaffReportItem]int)

	for _, u := range staffUsers {
		name := u.Username
		if u.FirstName != "" || u.LastName != "" {
			name = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
		item := &StaffReportItem{
			StaffID:   u.ID,
			StaffName: name,
			Username:  u.Username,
		}
		staffMap[u.ID] = item
		staffList = append(staffList, item)
	}

	unassignedItem := &StaffReportItem{
		StaffID:   0,
		StaffName: "Belum Diklaim / Pool Departemen",
		Username:  "-",
	}

	// 2. Ambil tiket untuk departemen ini pada bulan terpilih
	var tickets []models.Ticket
	config.DB.Preload("AssignedTo").
		Where("department_id = ?", deptID).
		Where("((created_at >= ? AND created_at < ?) OR (created_at >= ? AND created_at < ?) OR (EXTRACT(MONTH FROM created_at) = ? AND EXTRACT(YEAR FROM created_at) = ?))",
			startMonthWIB, endMonthWIB, startMonthUTC, endMonthUTC, month, year).
		Order("created_at ASC").
		Find(&tickets)

	// 3. Ambil rating untuk tiket
	var ticketIDs []uint
	for _, t := range tickets {
		ticketIDs = append(ticketIDs, t.ID)
	}
	ratingMap := make(map[uint]int)
	if len(ticketIDs) > 0 {
		var ratings []models.TicketRating
		config.DB.Where("ticket_id IN ?", ticketIDs).Find(&ratings)
		for _, r := range ratings {
			ratingMap[r.TicketID] = r.Rating
		}
	}

	// 4. Agregasi metrik per staff & summary departemen
	var deptSummary DepartmentReportItem
	deptSummary.DepartmentID = deptID
	deptSummary.DepartmentName = dept.Name
	var deptRatingScoreTotal int

	for _, t := range tickets {
		var target *StaffReportItem
		if t.AssignedToID != nil && *t.AssignedToID > 0 {
			if s, ok := staffMap[*t.AssignedToID]; ok {
				target = s
			} else {
				// Staff luar atau mantan staff
				name := fmt.Sprintf("User #%d", *t.AssignedToID)
				username := fmt.Sprintf("user_%d", *t.AssignedToID)
				if t.AssignedTo != nil {
					username = t.AssignedTo.Username
					if t.AssignedTo.FirstName != "" || t.AssignedTo.LastName != "" {
						name = strings.TrimSpace(t.AssignedTo.FirstName + " " + t.AssignedTo.LastName)
					} else {
						name = t.AssignedTo.Username
					}
				}
				item := &StaffReportItem{
					StaffID:   *t.AssignedToID,
					StaffName: name,
					Username:  username,
				}
				staffMap[*t.AssignedToID] = item
				staffList = append(staffList, item)
				target = item
			}
		} else {
			target = unassignedItem
		}

		target.TotalTickets++
		deptSummary.TotalTickets++

		if t.Status == models.StatusClosed {
			target.ClosedTickets++
			deptSummary.ClosedTickets++
		} else {
			target.OpenTickets++
			deptSummary.OpenTickets++
		}

		isRespBreach := false
		if t.FirstResponseMet != nil && !*t.FirstResponseMet {
			isRespBreach = true
		} else if t.FirstResponseAt == nil && t.FirstResponseDeadline != nil && now.After(*t.FirstResponseDeadline) {
			isRespBreach = true
		}
		if isRespBreach {
			target.FirstResponseBreached++
			deptSummary.FirstResponseBreached++
		}

		isResBreach := false
		if t.ResolutionDeadline != nil {
			if t.Status == models.StatusClosed && t.UpdatedAt.After(*t.ResolutionDeadline) {
				isResBreach = true
			} else if t.Status != models.StatusClosed && now.After(*t.ResolutionDeadline) {
				isResBreach = true
			}
		}
		if isResBreach {
			target.ResolutionBreached++
			deptSummary.ResolutionBreached++
		}

		if !isRespBreach && !isResBreach {
			target.SLAMetCount++
			deptSummary.SLAMetCount++
		}

		if score, ok := ratingMap[t.ID]; ok && score > 0 {
			target.RatedCount++
			staffRatingScores[target] += score

			deptSummary.RatedCount++
			deptRatingScoreTotal += score
		}
	}

	// 5. Hitung SLA Met Rate dan Avg Rating
	for _, s := range staffList {
		if s.TotalTickets > 0 {
			s.SLAMetRate = math.Round((float64(s.SLAMetCount)/float64(s.TotalTickets)*100)*10) / 10
		}
		if s.RatedCount > 0 {
			s.AvgRating = math.Round((float64(staffRatingScores[s])/float64(s.RatedCount))*10) / 10
		}
	}
	if unassignedItem.TotalTickets > 0 {
		unassignedItem.SLAMetRate = math.Round((float64(unassignedItem.SLAMetCount)/float64(unassignedItem.TotalTickets)*100)*10) / 10
		if unassignedItem.RatedCount > 0 {
			unassignedItem.AvgRating = math.Round((float64(staffRatingScores[unassignedItem])/float64(unassignedItem.RatedCount))*10) / 10
		}
		report.UnassignedItem = unassignedItem
	}

	if deptSummary.TotalTickets > 0 {
		deptSummary.SLAMetRate = math.Round((float64(deptSummary.SLAMetCount)/float64(deptSummary.TotalTickets)*100)*10) / 10
	}
	if deptSummary.RatedCount > 0 {
		deptSummary.AvgRating = math.Round((float64(deptRatingScoreTotal)/float64(deptSummary.RatedCount))*10) / 10
	}

	// Urutkan staff berdasarkan total tiket terbanyak, lalu nama
	sort.Slice(staffList, func(i, j int) bool {
		if staffList[i].TotalTickets != staffList[j].TotalTickets {
			return staffList[i].TotalTickets > staffList[j].TotalTickets
		}
		return staffList[i].StaffName < staffList[j].StaffName
	})

	var resultStaffList []StaffReportItem
	for _, s := range staffList {
		resultStaffList = append(resultStaffList, *s)
	}

	report.Summary = deptSummary
	report.StaffList = resultStaffList

	logging.AdminReports.Info("GetDepartmentStaffReport completed",
		"department_id", deptID,
		"staff_count", len(resultStaffList),
		"has_unassigned", report.UnassignedItem != nil,
		"total_tickets", deptSummary.TotalTickets,
		"open_tickets", deptSummary.OpenTickets,
		"closed_tickets", deptSummary.ClosedTickets,
	)

	return report, nil
}

