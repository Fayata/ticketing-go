package services

import (
	"fmt"
	"math"
	"sort"
	"time"

	"ticketing/config"
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
	GeneratedAtFormatted   string
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

	report := &MonthlyReportData{
		Filter:               filter,
		MonthName:            GetMonthName(filter.Month),
		Year:                 filter.Year,
		PeriodLabel:          fmt.Sprintf("%s %d", GetMonthName(filter.Month), filter.Year),
		MonthOptions:         s.GetMonthOptions(filter.Month, filter.Year),
		SelectedCompanyID:    selectedCID,
		GeneratedAtFormatted: now.Format("02 Jan 2006, 15:04"),
	}

	if config.DB == nil {
		return report, nil
	}

	// 1. Ambil daftar semua perusahaan aktif untuk opsi filter dan master data
	var allCompanies []models.Company
	config.DB.Order("name ASC").Find(&allCompanies)
	report.AllCompanies = allCompanies

	// 2. Ambil master departemen
	var allDepts []models.Department
	config.DB.Preload("Company").Order("name ASC").Find(&allDepts)

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

	// Inisialisasi awal Master Perusahaan & Departemen agar selalu tampil di tabel bahkan saat 0 tiket
	for _, comp := range targetCompanies {
		cID := comp.ID
		if !companySeen[cID] {
			companySeen[cID] = true
			companyOrder = append(companyOrder, cID)
			companyMeta[cID] = struct{ name, code string }{name: comp.Name, code: comp.Code}
		}

		// Daftarkan semua departemen di bawah perusahaan ini
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

	// Daftarkan departemen tanpa perusahaan jika sedang melihat semua perusahaan
	if filter.CompanyID == nil || *filter.CompanyID == 0 {
		for _, dept := range allDepts {
			if dept.CompanyID == nil || *dept.CompanyID == 0 {
				if !companySeen[0] {
					companySeen[0] = true
					companyOrder = append(companyOrder, 0)
					companyMeta[0] = struct{ name, code string }{name: "Umum / Tanpa Perusahaan", code: "DEFAULT"}
				}
				key := deptKey{CompanyID: 0, DepartmentID: dept.ID}
				if _, exists := deptStatsMap[key]; !exists {
					deptStatsMap[key] = &deptStats{
						companyID:      0,
						companyName:    "Umum / Tanpa Perusahaan",
						companyCode:    "DEFAULT",
						departmentID:   dept.ID,
						departmentName: dept.Name,
					}
				}
			}
		}
	}

	// 3. Query semua tiket pada rentang bulan ini (kompatibel lintas timezone)
	loc := now.Location()
	startOfMonth := time.Date(filter.Year, time.Month(filter.Month), 1, 0, 0, 0, 0, loc)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	startOfMonthUTC := time.Date(filter.Year, time.Month(filter.Month), 1, 0, 0, 0, 0, time.UTC)
	endOfMonthUTC := startOfMonthUTC.AddDate(0, 1, 0)

	query := config.DB.Model(&models.Ticket{}).
		Preload("Company").
		Preload("Department").
		Preload("Department.Company").
		Where("((created_at >= ? AND created_at < ?) OR (created_at >= ? AND created_at < ?) OR (EXTRACT(YEAR FROM created_at) = ? AND EXTRACT(MONTH FROM created_at) = ?)) AND deleted_at IS NULL",
			startOfMonth, endOfMonth, startOfMonthUTC, endOfMonthUTC, filter.Year, filter.Month)

	if filter.CompanyID != nil && *filter.CompanyID > 0 {
		query = query.Where("company_id = ?", *filter.CompanyID)
	}

	var tickets []models.Ticket
	if err := query.Order("created_at ASC").Find(&tickets).Error; err != nil {
		return nil, err
	}

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
			if t.Company != nil {
				cName = t.Company.Name
				cCode = t.Company.Code
			} else if meta, ok := companyMeta[cID]; ok {
				cName = meta.name
				cCode = meta.code
			}
		} else if t.Department != nil && t.Department.CompanyID != nil && *t.Department.CompanyID > 0 {
			cID = *t.Department.CompanyID
			if t.Department.Company != nil {
				cName = t.Department.Company.Name
				cCode = t.Department.Company.Code
			} else if meta, ok := companyMeta[cID]; ok {
				cName = meta.name
				cCode = meta.code
			}
		}

		if filter.CompanyID != nil && *filter.CompanyID > 0 && cID != *filter.CompanyID {
			continue
		}

		dID := uint(0)
		dName := "Umum / Tanpa Departemen"
		if t.DepartmentID != nil && *t.DepartmentID > 0 {
			dID = *t.DepartmentID
			if t.Department != nil {
				dName = t.Department.Name
			}
		}

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

	return report, nil
}
