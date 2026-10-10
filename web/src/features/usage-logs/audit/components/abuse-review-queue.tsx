import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
	getAbuseReviews,
	getAbuseReviewSummary,
	updateAbuseReview,
  type AbuseReview,
  type AbuseReviewStatus,
} from '../api'

function formatReviewTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

export function AbuseReviewQueue() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<AbuseReviewStatus>('pending')
  const [action, setAction] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [notes, setNotes] = useState<Record<number, string>>({})
  const query = useQuery({
    queryKey: ['abuse-reviews', status, action, page, pageSize],
    queryFn: () => getAbuseReviews({ p: page, page_size: pageSize, status, action: action || undefined }),
    retry: false,
  })
  const summaryQuery = useQuery({
    queryKey: ['abuse-review-summary'],
    queryFn: getAbuseReviewSummary,
    retry: false,
    refetchInterval: 60_000,
  })
  const mutation = useMutation({
    mutationFn: ({ id, nextStatus }: { id: number; nextStatus: Exclude<AbuseReviewStatus, 'pending'> }) =>
      updateAbuseReview(id, nextStatus, notes[id] ?? ''),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['abuse-reviews'] })
      void queryClient.invalidateQueries({ queryKey: ['abuse-review-summary'] })
    },
  })

  const columns = [
    {
      accessorKey: 'action',
      header: t('Signal'),
      cell: ({ row }: { row: { original: AbuseReview } }) => (
        <div className='space-y-1'>
          <div className='font-medium'>{row.original.action}</div>
          <div className='text-muted-foreground text-xs'>
            {t('Event')} {row.original.event_id}
          </div>
        </div>
      ),
    },
    {
      accessorKey: 'risk_score',
      header: t('Risk score'),
      cell: ({ row }: { row: { original: AbuseReview } }) => (
        <div className='space-y-1'>
          <div>{row.original.risk_score}</div>
          <div className='text-muted-foreground text-xs'>
            {row.original.disposition ?? 'observe'}
          </div>
        </div>
      ),
    },
    {
      accessorKey: 'created_at',
      header: t('Created'),
      cell: ({ row }: { row: { original: AbuseReview } }) => (
        <div className='space-y-1'>
          <div>{formatReviewTime(row.original.created_at)}</div>
          <div className='text-muted-foreground text-xs'>
            {t('User')} {row.original.user_id} / {t('Token')} {row.original.token_id}
          </div>
        </div>
      ),
    },
    {
      accessorKey: 'evidence_json',
      header: t('Redacted evidence'),
      cell: ({ row }: { row: { original: AbuseReview } }) => (
        <pre className='bg-muted max-w-80 overflow-x-auto rounded-md p-2 text-xs whitespace-pre-wrap'>
          {row.original.evidence_json || '{}'}
        </pre>
      ),
    },
    {
      id: 'review',
      header: t('Review'),
      cell: ({ row }: { row: { original: AbuseReview } }) => {
        const review = row.original
        if (review.status !== 'pending') {
          return (
            <div className='space-y-1 text-xs'>
              <div>{review.status}</div>
              {review.review_note && <div className='text-muted-foreground'>{review.review_note}</div>}
            </div>
          )
        }
        return (
          <div className='min-w-56 space-y-2'>
            <Textarea
              aria-label={`${t('Review note')} ${review.id}`}
              className='min-h-16 text-xs'
              placeholder={t('Review note')}
              value={notes[review.id] ?? ''}
              onChange={(event) =>
                setNotes((previous) => ({ ...previous, [review.id]: event.target.value }))
              }
            />
            <div className='flex gap-2'>
              <Button
                size='sm'
                disabled={mutation.isPending}
                onClick={() => mutation.mutate({ id: review.id, nextStatus: 'resolved' })}
              >
                {t('Resolve')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                disabled={mutation.isPending}
                onClick={() => mutation.mutate({ id: review.id, nextStatus: 'false_positive' })}
              >
                {t('False positive')}
              </Button>
            </div>
          </div>
        )
      },
    },
  ]
  const { table } = useDataTable({
    columns,
    data: query.data?.items ?? [],
    getRowId: (entry) => String(entry.id),
    totalCount: query.data?.total ?? 0,
    pagination: { pageIndex: page - 1, pageSize },
    onPaginationChange: (updater) => {
      const current = { pageIndex: page - 1, pageSize }
      const next = typeof updater === 'function' ? updater(current) : updater
      setPage(next.pageIndex + 1)
    },
    enableRowSelection: false,
    enableSorting: false,
    manualPagination: true,
  })

  return (
    <div className='flex h-full min-h-0 flex-col'>
      {summaryQuery.data && (
        <div className='mb-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-3' aria-label={t('Abuse review summary')}>
          {[
            [t('Pending'), summaryQuery.data.reviews.pending],
            [t('Resolved'), summaryQuery.data.reviews.resolved],
            [t('False positive'), summaryQuery.data.reviews.false_positive],
            [t('Last 24 hours'), summaryQuery.data.reviews.last_24_hours],
            [t('Review priority'), summaryQuery.data.reviews.review_priority],
            [t('Manual priority'), summaryQuery.data.reviews.manual_priority],
          ].map(([label, value]) => (
            <div key={String(label)} className='border-border bg-muted/20 rounded-md border px-3 py-2'>
              <div className='text-muted-foreground'>{label}</div>
              <div className='mt-1 text-lg font-semibold'>{value}</div>
            </div>
          ))}
          <div className='border-border bg-muted/20 col-span-2 rounded-md border px-3 py-2 sm:col-span-3'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <span className='text-muted-foreground'>{t('External content audit shadow')}</span>
              <span className={summaryQuery.data.content_audit.enabled && summaryQuery.data.content_audit.configured ? 'text-emerald-600' : 'text-muted-foreground'}>
                {summaryQuery.data.content_audit.enabled && summaryQuery.data.content_audit.configured ? t('Configured') : t('Disabled')}
              </span>
            </div>
            <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-4 gap-y-1'>
              <span>{t('Requests')} {summaryQuery.data.content_audit.requests}</span>
              <span>{t('Flagged')} {summaryQuery.data.content_audit.flagged}</span>
              <span>{t('Errors')} {summaryQuery.data.content_audit.errors}</span>
              <span>{t('Dropped')} {summaryQuery.data.content_audit.dropped}</span>
              {summaryQuery.data.content_audit.model && <span>{summaryQuery.data.content_audit.model}</span>}
            </div>
          </div>
        </div>
      )}
      {query.isError && (
        <Alert variant='destructive' className='mb-3'>
          <AlertDescription>{t('Failed to load abuse reviews')}</AlertDescription>
        </Alert>
      )}
      {summaryQuery.isError && (
        <Alert variant='destructive' className='mb-3'>
          <AlertDescription>{t('Failed to load abuse review summary')}</AlertDescription>
        </Alert>
      )}
      <div className='mb-3 flex max-w-xl flex-wrap items-center gap-2'>
        <Combobox
          aria-label={t('Review status')}
          value={status}
          options={[
            { value: 'pending', label: t('Pending') },
            { value: 'resolved', label: t('Resolved') },
            { value: 'false_positive', label: t('False positive') },
          ]}
          onValueChange={(value) => {
            if (value === 'pending' || value === 'resolved' || value === 'false_positive') {
              setStatus(value)
              setPage(1)
            }
          }}
        />
        <Input
          aria-label={t('Signal filter')}
          className='w-64'
          placeholder={t('Filter action, e.g. abuse.content_audit')}
          value={action}
          onChange={(event) => {
            setAction(event.target.value)
            setPage(1)
          }}
        />
      </div>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={query.isPending}
        isFetching={query.isFetching}
        emptyTitle={t('No abuse reviews')}
        className='min-h-0 flex-1'
        applyHeaderSize
        getColumnClassName={() => 'py-2'}
        tableClassName='[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
      />
    </div>
  )
}
