import re

with open('utils/render.go', 'r', encoding='utf-8') as f:
    content = f.read()

if '"admin_search_results":' not in content:
    content = content.replace(
        '"admin_dashboard":       {"templates/admin/admin_base.html", "templates/admin/admin_dashboard.html"},',
        '"admin_dashboard":       {"templates/admin/admin_base.html", "templates/admin/admin_dashboard.html"},\n\t\t"admin_search_results":  {"templates/admin/admin_base.html", "templates/admin/search_results.html"},'
    )
    with open('utils/render.go', 'w', encoding='utf-8') as f:
        f.write(content)
