/**
 * Inisialisasi chart dashboard staff: tren, bulanan, donut.
 * Data diambil dari <script type="application/json" id="staffTrendData"> dll.
 */
(function () {
	function parseJson(id, fallback) {
		var el = document.getElementById(id);
		if (!el || !el.textContent) return fallback || [];
		try {
			return JSON.parse(el.textContent);
		} catch (e) {
			return fallback || [];
		}
	}

	function initCharts() {
		if (typeof Chart === 'undefined') return;

		var trendData = parseJson('staffTrendData', []);
		var monthlyData = parseJson('staffMonthlyData', []);
		var donutData = parseJson('staffDonutData', []);

		var colors = {
			blue: 'rgb(59, 130, 246)',
			green: 'rgb(16, 185, 129)',
			gray: 'rgb(243, 244, 246)'
		};

		var ctx = document.getElementById('staffChartTrend');
		if (ctx) {
			var labels = trendData.length ? trendData.map(function (d) { return d.d; }) : [];
			var ambil = trendData.length ? trendData.map(function (d) { return d.ambil; }) : [];
			var selesai = trendData.length ? trendData.map(function (d) { return d.selesai; }) : [];
			new Chart(ctx, {
				type: 'line',
				data: {
					labels: labels,
					datasets: [
						{ label: 'Tiket Diambil', data: ambil, borderColor: colors.blue, backgroundColor: 'rgba(59,130,246,0.15)', fill: true, tension: 0.3 },
						{ label: 'Tiket Diselesaikan', data: selesai, borderColor: colors.green, backgroundColor: 'rgba(16,185,129,0.15)', fill: true, tension: 0.3 }
					]
				},
				options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'top' } }, scales: { x: { grid: { display: false } }, y: { beginAtZero: true, grid: { color: colors.gray } } } }
			});
		}

		ctx = document.getElementById('staffChartMonthly');
		if (ctx) {
			var barLabels = monthlyData.length ? monthlyData.map(function (d) { return d.b; }) : ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Ags', 'Sep', 'Okt', 'Nov', 'Des'];
			var barData = monthlyData.length ? monthlyData.map(function (d) { return d.v; }) : [];
			var barColors = monthlyData.length ? monthlyData.map(function (d) { return d.v > 0 ? 'rgb(16, 185, 129)' : colors.gray; }) : [];
			new Chart(ctx, {
				type: 'bar',
				data: {
					labels: barLabels,
					datasets: [{ label: 'Tiket Selesai', data: barData, backgroundColor: barColors }]
				},
				options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { x: { grid: { display: false } }, y: { beginAtZero: true } } }
			});
		}

		ctx = document.getElementById('staffChartDonut');
		if (ctx) {
			new Chart(ctx, {
				type: 'doughnut',
				data: {
					labels: donutData.length ? donutData.map(function (d) { return d.name; }) : [],
					datasets: [{
						data: donutData.length ? donutData.map(function (d) { return d.value; }) : [],
						backgroundColor: donutData.length ? donutData.map(function (d) { return d.color; }) : []
					}]
				},
				options: { responsive: true, maintainAspectRatio: false, cutout: '60%', plugins: { legend: { display: false } } }
			});
		}
	}

	if (document.readyState === 'loading') {
		document.addEventListener('DOMContentLoaded', initCharts);
	} else {
		initCharts();
	}
})();
