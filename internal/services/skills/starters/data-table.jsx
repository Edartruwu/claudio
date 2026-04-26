// data-table.jsx — DataTable + useTableState
// No imports. React is global. CSS injected once via <style id="dt-styles">.

(function () {
  // ── CSS injection ────────────────────────────────────────────────────────────
  if (!document.getElementById("dt-styles")) {
    const style = document.createElement("style");
    style.id = "dt-styles";
    style.textContent = `
@keyframes dt-shimmer {
  0%   { background-position: -400px 0; }
  100% { background-position: 400px 0; }
}
.dt-wrap { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; font-size: 14px; color: #1e293b; }
.dt-toolbar { display: flex; align-items: center; gap: 12px; padding: 12px 0; }
.dt-search { flex: 1; padding: 8px 12px; border: 1px solid #e2e8f0; border-radius: 6px; font-size: 14px; outline: none; }
.dt-search:focus { border-color: #3b82f6; box-shadow: 0 0 0 2px rgba(59,130,246,0.15); }
.dt-scroll { overflow: auto; border: 1px solid #e2e8f0; border-radius: 8px; }
.dt-table { width: 100%; border-collapse: collapse; }
.dt-thead { position: sticky; top: 0; z-index: 2; background: #f8fafc; }
.dt-th { padding: 10px 14px; font-size: 11px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: #64748b; white-space: nowrap; user-select: none; border-bottom: 1px solid #e2e8f0; position: relative; }
.dt-th.sortable { cursor: pointer; }
.dt-th.sortable:hover { background: #f1f5f9; color: #334155; }
.dt-th .dt-sort-icon { margin-left: 4px; opacity: 0.4; }
.dt-th .dt-sort-icon.active { opacity: 1; color: #3b82f6; }
.dt-th .dt-resize-handle { position: absolute; right: 0; top: 0; bottom: 0; width: 4px; cursor: col-resize; z-index: 1; }
.dt-th .dt-resize-handle:hover, .dt-th .dt-resize-handle.resizing { background: #3b82f6; }
.dt-td { padding: 14px 14px; border-bottom: 1px solid #e2e8f0; vertical-align: middle; }
.dt-compact .dt-td { padding: 8px 14px; }
.dt-tr { transition: background 120ms ease; }
.dt-tr:hover .dt-td { background: #f8fafc; }
.dt-tr.selected .dt-td { background: #eff6ff; }
.dt-tr.clickable { cursor: pointer; }
.dt-striped .dt-tr:nth-child(even) .dt-td { background: #f8fafc; }
.dt-striped .dt-tr:nth-child(even):hover .dt-td { background: #f1f5f9; }
.dt-td.align-center, .dt-th.align-center { text-align: center; }
.dt-td.align-right,  .dt-th.align-right  { text-align: right; }
.dt-checkbox { width: 16px; height: 16px; cursor: pointer; accent-color: #3b82f6; }
.dt-empty { text-align: center; padding: 48px 24px; color: #94a3b8; }
.dt-empty-icon { font-size: 32px; margin-bottom: 8px; }
.dt-skel { display: inline-block; border-radius: 4px; background: linear-gradient(90deg, #e2e8f0 25%, #f1f5f9 50%, #e2e8f0 75%); background-size: 400px 100%; animation: dt-shimmer 1.4s infinite; height: 14px; }
.dt-footer { display: flex; align-items: center; justify-content: space-between; padding: 12px 0; gap: 8px; flex-wrap: wrap; }
.dt-info { font-size: 13px; color: #64748b; }
.dt-pagination { display: flex; align-items: center; gap: 4px; }
.dt-page-btn { padding: 4px 10px; border: 1px solid #e2e8f0; background: #fff; border-radius: 5px; font-size: 13px; cursor: pointer; color: #374151; min-width: 32px; }
.dt-page-btn:hover:not(:disabled) { background: #f1f5f9; border-color: #cbd5e1; }
.dt-page-btn.current { background: #3b82f6; border-color: #3b82f6; color: #fff; font-weight: 600; }
.dt-page-btn:disabled { opacity: 0.4; cursor: default; }
.dt-page-size { padding: 4px 8px; border: 1px solid #e2e8f0; border-radius: 5px; font-size: 13px; background: #fff; cursor: pointer; }
`;
    document.head.appendChild(style);
  }

  // ── Helpers ──────────────────────────────────────────────────────────────────
  const { useState, useMemo, useCallback, useRef } = React;

  function SortIcon({ dir }) {
    if (!dir) return React.createElement("span", { className: "dt-sort-icon" }, "⇅");
    return React.createElement("span", { className: "dt-sort-icon active" }, dir === "asc" ? "↑" : "↓");
  }

  function SkeletonRow({ cols, selectable, compact }) {
    const widths = [40, 60, 50, 70, 55, 65, 45];
    return React.createElement(
      "tr", { className: "dt-tr" + (compact ? " dt-compact" : "") },
      selectable && React.createElement("td", { className: "dt-td", style: { width: 40 } },
        React.createElement("span", { className: "dt-skel", style: { width: 16, height: 16, borderRadius: 3 } })
      ),
      cols.map((col, i) =>
        React.createElement("td", { key: col.key, className: "dt-td" },
          React.createElement("span", { className: "dt-skel", style: { width: `${widths[i % widths.length]}%` } })
        )
      )
    );
  }

  // ── useTableState ────────────────────────────────────────────────────────────
  function useTableState(data = [], options = {}) {
    const [sort, setSort] = useState({ key: null, dir: null });
    const [filter, setFilter] = useState("");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSizeState] = useState(options.pageSize || 10);
    const [selected, setSelected] = useState([]);

    const filtered = useMemo(() => {
      if (!filter.trim()) return data;
      const q = filter.toLowerCase();
      return data.filter(row =>
        Object.values(row).some(v =>
          v != null && String(v).toLowerCase().includes(q)
        )
      );
    }, [data, filter]);

    const sorted = useMemo(() => {
      if (!sort.key || !sort.dir) return filtered;
      return [...filtered].sort((a, b) => {
        const av = a[sort.key], bv = b[sort.key];
        const cmp = av == null ? -1 : bv == null ? 1 :
          typeof av === "number" && typeof bv === "number"
            ? av - bv
            : String(av).localeCompare(String(bv));
        return sort.dir === "asc" ? cmp : -cmp;
      });
    }, [filtered, sort]);

    const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));

    const rows = useMemo(() => {
      const start = (page - 1) * pageSize;
      return sorted.slice(start, start + pageSize);
    }, [sorted, page, pageSize]);

    const setPageSize = useCallback((sz) => {
      setPageSizeState(sz);
      setPage(1);
    }, []);

    // Reset page when filter/sort changes
    const prevFilter = useRef(filter);
    const prevSort = useRef(sort);
    if (prevFilter.current !== filter || prevSort.current !== sort) {
      prevFilter.current = filter;
      prevSort.current = sort;
      if (page !== 1) setPage(1);
    }

    return { rows, sort, setSort, filter, setFilter, page, setPage,
             pageSize, setPageSize, selected, setSelected, totalPages,
             total: sorted.length };
  }

  // ── DataTable ────────────────────────────────────────────────────────────────
  function DataTable({
    columns = [],
    data = [],
    keyField = "id",
    selectable = false,
    onSelectionChange,
    pageSize: pageSizeProp = 10,
    pageSizeOptions = [10, 25, 50],
    loading = false,
    emptyMessage = "No data available",
    onRowClick,
    stickyHeader = true,
    maxHeight = "500px",
    striped = false,
    compact = false,
  }) {
    const state = useTableState(data, { pageSize: pageSizeProp });
    const { rows, sort, setSort, filter, setFilter, page, setPage,
            pageSize, setPageSize, selected, setSelected, totalPages, total } = state;

    // Column widths for resize
    const [colWidths, setColWidths] = useState({});
    const resizingRef = useRef(null);

    const handleSortClick = useCallback((col) => {
      if (!col.sortable) return;
      setSort(prev => {
        if (prev.key !== col.key) return { key: col.key, dir: "asc" };
        if (prev.dir === "asc") return { key: col.key, dir: "desc" };
        return { key: null, dir: null };
      });
    }, [setSort]);

    // Selection helpers
    const pageKeys = rows.map(r => r[keyField]);
    const allPageSelected = pageKeys.length > 0 && pageKeys.every(k => selected.includes(k));
    const somePageSelected = pageKeys.some(k => selected.includes(k)) && !allPageSelected;

    const toggleAll = useCallback(() => {
      const next = allPageSelected
        ? selected.filter(k => !pageKeys.includes(k))
        : [...new Set([...selected, ...pageKeys])];
      setSelected(next);
      onSelectionChange?.(next);
    }, [allPageSelected, pageKeys, selected, setSelected, onSelectionChange]);

    const toggleRow = useCallback((key) => {
      const next = selected.includes(key)
        ? selected.filter(k => k !== key)
        : [...selected, key];
      setSelected(next);
      onSelectionChange?.(next);
    }, [selected, setSelected, onSelectionChange]);

    // Resize drag
    const startResize = useCallback((e, colKey) => {
      e.preventDefault();
      const th = e.target.closest("th");
      const startX = e.clientX;
      const startW = th.offsetWidth;
      resizingRef.current = colKey;

      const onMove = (mv) => {
        const delta = mv.clientX - startX;
        setColWidths(prev => ({ ...prev, [colKey]: Math.max(60, startW + delta) }));
      };
      const onUp = () => {
        resizingRef.current = null;
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    }, []);

    // Pagination window (5 pages)
    const pageNums = useMemo(() => {
      const half = 2, pages = [];
      let start = Math.max(1, page - half);
      let end = Math.min(totalPages, start + 4);
      if (end - start < 4) start = Math.max(1, end - 4);
      for (let i = start; i <= end; i++) pages.push(i);
      return pages;
    }, [page, totalPages]);

    const startRow = total === 0 ? 0 : (page - 1) * pageSize + 1;
    const endRow = Math.min(page * pageSize, total);

    // ── Render ─────────────────────────────────────────────────────────────────
    const tableClasses = [
      "dt-table",
      striped ? "dt-striped" : "",
      compact ? "dt-compact" : "",
    ].filter(Boolean).join(" ");

    const scrollStyle = {
      maxHeight: stickyHeader ? maxHeight : undefined,
      overflow: stickyHeader ? "auto" : undefined,
    };

    return React.createElement(
      "div", { className: "dt-wrap" },

      // Toolbar: search
      React.createElement("div", { className: "dt-toolbar" },
        React.createElement("input", {
          className: "dt-search",
          type: "search",
          placeholder: "Search…",
          value: filter,
          onChange: e => setFilter(e.target.value),
          "aria-label": "Search table",
        })
      ),

      // Table scroll container
      React.createElement(
        "div", { className: "dt-scroll", style: scrollStyle },
        React.createElement(
          "table", { className: tableClasses, role: "grid" },

          // Header
          React.createElement(
            "thead", { className: "dt-thead" },
            React.createElement(
              "tr", null,
              selectable && React.createElement(
                "th", { className: "dt-th", style: { width: 40 } },
                React.createElement("input", {
                  type: "checkbox",
                  className: "dt-checkbox",
                  checked: allPageSelected,
                  ref: el => { if (el) el.indeterminate = somePageSelected; },
                  onChange: toggleAll,
                  "aria-label": "Select all on page",
                })
              ),
              columns.map(col =>
                React.createElement(
                  "th", {
                    key: col.key,
                    className: [
                      "dt-th",
                      col.sortable ? "sortable" : "",
                      col.align ? `align-${col.align}` : "",
                    ].filter(Boolean).join(" "),
                    style: { width: colWidths[col.key] || col.width || undefined },
                    onClick: () => handleSortClick(col),
                    "aria-sort": sort.key === col.key
                      ? (sort.dir === "asc" ? "ascending" : "descending")
                      : "none",
                  },
                  col.label,
                  col.sortable && React.createElement(SortIcon, {
                    dir: sort.key === col.key ? sort.dir : null
                  }),
                  // Resize handle
                  React.createElement("div", {
                    className: "dt-resize-handle" + (resizingRef.current === col.key ? " resizing" : ""),
                    onMouseDown: e => startResize(e, col.key),
                    onClick: e => e.stopPropagation(),
                    title: "Drag to resize",
                  })
                )
              )
            )
          ),

          // Body
          React.createElement(
            "tbody", null,
            loading
              ? Array.from({ length: pageSize }, (_, i) =>
                  React.createElement(SkeletonRow, { key: i, cols: columns, selectable, compact })
                )
              : rows.length === 0
                ? React.createElement(
                    "tr", null,
                    React.createElement(
                      "td", {
                        className: "dt-td dt-empty",
                        colSpan: columns.length + (selectable ? 1 : 0),
                      },
                      React.createElement("div", { className: "dt-empty-icon" }, "📭"),
                      React.createElement("div", null, emptyMessage)
                    )
                  )
                : rows.map(row => {
                    const key = row[keyField];
                    const isSelected = selected.includes(key);
                    return React.createElement(
                      "tr", {
                        key,
                        className: [
                          "dt-tr",
                          isSelected ? "selected" : "",
                          onRowClick ? "clickable" : "",
                        ].filter(Boolean).join(" "),
                        onClick: onRowClick ? () => onRowClick(row) : undefined,
                      },
                      selectable && React.createElement(
                        "td", {
                          className: "dt-td",
                          style: { width: 40 },
                          onClick: e => { e.stopPropagation(); toggleRow(key); },
                        },
                        React.createElement("input", {
                          type: "checkbox",
                          className: "dt-checkbox",
                          checked: isSelected,
                          onChange: () => toggleRow(key),
                          "aria-label": `Select row ${key}`,
                        })
                      ),
                      columns.map(col =>
                        React.createElement(
                          "td", {
                            key: col.key,
                            className: ["dt-td", col.align ? `align-${col.align}` : ""].filter(Boolean).join(" "),
                            style: { width: colWidths[col.key] || col.width || undefined },
                          },
                          col.render ? col.render(row[col.key], row) : row[col.key]
                        )
                      )
                    );
                  })
          )
        )
      ),

      // Footer: info + pagination
      React.createElement(
        "div", { className: "dt-footer" },

        // Showing X–Y of Z
        React.createElement("span", { className: "dt-info" },
          loading
            ? "Loading…"
            : total === 0
              ? "No results"
              : `Showing ${startRow}–${endRow} of ${total}`
        ),

        // Right side: page-size selector + pagination
        React.createElement(
          "div", { style: { display: "flex", alignItems: "center", gap: 12 } },

          React.createElement(
            "select", {
              className: "dt-page-size",
              value: pageSize,
              onChange: e => setPageSize(Number(e.target.value)),
              "aria-label": "Rows per page",
            },
            pageSizeOptions.map(sz =>
              React.createElement("option", { key: sz, value: sz }, `${sz} / page`)
            )
          ),

          React.createElement(
            "div", { className: "dt-pagination", role: "navigation", "aria-label": "Pagination" },
            React.createElement("button", {
              className: "dt-page-btn",
              disabled: page <= 1,
              onClick: () => setPage(p => p - 1),
              "aria-label": "Previous page",
            }, "‹"),
            pageNums.map(n =>
              React.createElement("button", {
                key: n,
                className: "dt-page-btn" + (n === page ? " current" : ""),
                onClick: () => setPage(n),
                "aria-label": `Page ${n}`,
                "aria-current": n === page ? "page" : undefined,
              }, n)
            ),
            React.createElement("button", {
              className: "dt-page-btn",
              disabled: page >= totalPages,
              onClick: () => setPage(p => p + 1),
              "aria-label": "Next page",
            }, "›")
          )
        )
      )
    );
  }

  // ── Exports ──────────────────────────────────────────────────────────────────
  window.DataTable = DataTable;
  window.useTableState = useTableState;
})();
