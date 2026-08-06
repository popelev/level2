import { useEffect, useState } from 'react'

export default function TagPager({ page, totalPages, onPageChange }) {
  const [draft, setDraft] = useState(String(page))

  useEffect(() => {
    setDraft(String(page))
  }, [page])

  const go = (p) => {
    const n = Math.min(totalPages, Math.max(1, p))
    onPageChange(n)
  }

  const commitDraft = () => {
    const n = Number.parseInt(draft, 10)
    if (Number.isFinite(n)) go(n)
    else setDraft(String(page))
  }

  return (
    <div className="tag-pager" role="navigation" aria-label="Pagination">
      <button
        type="button"
        className="pager-nav"
        disabled={page <= 1}
        title="First page"
        onClick={() => go(1)}
      >
        |◀
      </button>
      <button
        type="button"
        className="pager-nav"
        disabled={page <= 1}
        title="Previous page"
        onClick={() => go(page - 1)}
      >
        ◀
      </button>
      <div className="pager-page">
        <input
          type="text"
          inputMode="numeric"
          className="pager-input"
          value={draft}
          aria-label="Page number"
          onChange={(e) => setDraft(e.target.value.replace(/\D/g, ''))}
          onBlur={commitDraft}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              commitDraft()
            }
          }}
        />
        <span className="pager-of muted">of {totalPages}</span>
      </div>
      <button
        type="button"
        className="pager-nav"
        disabled={page >= totalPages}
        title="Next page"
        onClick={() => go(page + 1)}
      >
        ▶
      </button>
      <button
        type="button"
        className="pager-nav"
        disabled={page >= totalPages}
        title="Last page"
        onClick={() => go(totalPages)}
      >
        ▶|
      </button>
    </div>
  )
}
