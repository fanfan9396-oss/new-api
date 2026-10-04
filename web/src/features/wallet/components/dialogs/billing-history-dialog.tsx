/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Search, Copy, Check, ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { handleServerError } from '@/lib/handle-server-error'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { useBillingHistory } from '../../hooks/use-billing-history'
import type { TopupRecord } from '../../types'
import { cancelWalletRefund, completeWalletRefund, startWalletRefund, isApiSuccess } from '../../api'
import {
  getStatusConfig,
  getPaymentMethodName,
  formatTimestamp,
} from '../../lib/billing'

interface BillingHistoryDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDataRefresh?: () => void | Promise<void>
}

export function BillingHistoryDialog({
  open,
  onOpenChange,
  onDataRefresh,
}: BillingHistoryDialogProps) {
  const { t } = useTranslation()
  const {
    records,
    total,
    page,
    pageSize,
    keyword,
    loading,
    completing,
    isAdmin,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleCompleteOrder,
  } = useBillingHistory({ enabled: open, onDataRefresh })

  const [confirmTradeNo, setConfirmTradeNo] = useState<string | null>(null)
  const [refundRecord, setRefundRecord] = useState<TopupRecord | null>(null)
  const [refundId, setRefundId] = useState<number | null>(null)
  const [refundReason, setRefundReason] = useState('')
  const [refundProof, setRefundProof] = useState('')
  const [deductQuota, setDeductQuota] = useState('')
  const [refundLoading, setRefundLoading] = useState(false)
  const { copyToClipboard, copiedText } = useCopyToClipboard({ notify: false })

  const totalPages = Math.ceil(total / pageSize)

  const handleConfirmComplete = async () => {
    if (confirmTradeNo) {
      const success = await handleCompleteOrder(confirmTradeNo)
      if (success) {
        setConfirmTradeNo(null)
      }
    }
  }

  const closeRefund = () => {
    setRefundRecord(null)
    setRefundId(null)
    setRefundReason('')
    setRefundProof('')
    setDeductQuota('')
  }

  const handleStartRefund = async () => {
    if (!refundRecord || !refundReason.trim()) return
    setRefundLoading(true)
    try {
      const response = await startWalletRefund({ user_id: refundRecord.user_id, trade_no: refundRecord.trade_no, reason: refundReason.trim() })
      if (!isApiSuccess(response) || !response.data) throw response
      setRefundId(response.data.id)
      toast.success(t('Refund processing started'))
    } catch (error) {
      handleServerError(error, t('Failed to start refund processing'))
    } finally { setRefundLoading(false) }
  }

  const handleCompleteRefund = async () => {
    const quota = Number.parseInt(deductQuota, 10)
    if (!refundId || !refundReason.trim() || !refundProof.trim() || !Number.isFinite(quota) || quota <= 0) return
    setRefundLoading(true)
    try {
      const response = await completeWalletRefund({ refund_id: refundId, deduct_quota: quota, reason: refundReason.trim(), proof_ref: refundProof.trim() })
      if (!isApiSuccess(response)) throw response
      toast.success(t('Refund processing completed'))
      closeRefund()
      await onDataRefresh?.()
    } catch (error) {
      handleServerError(error, t('Failed to complete refund processing'))
    } finally { setRefundLoading(false) }
  }

  const handleCancelRefund = async () => {
    if (!refundId || !refundReason.trim()) return
    setRefundLoading(true)
    try {
      const response = await cancelWalletRefund({ refund_id: refundId, reason: refundReason.trim() })
      if (!isApiSuccess(response)) throw response
      toast.success(t('Refund processing cancelled'))
      closeRefund()
      await onDataRefresh?.()
    } catch (error) {
      handleServerError(error, t('Failed to cancel refund processing'))
    } finally { setRefundLoading(false) }
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={t('Billing History')}
        description={t(
          'View your topup transaction records and payment history'
        )}
        contentClassName='flex max-h-(--dialog-available-height) flex-col overflow-hidden overscroll-contain max-sm:w-screen max-sm:max-w-none max-sm:rounded-none max-sm:p-4 sm:max-w-4xl'
        contentHeight='auto'
        bodyClassName='flex min-h-0 flex-1 flex-col space-y-3'
      >
        <div className='min-h-0 space-y-3'>
          {/* Search and Filter Bar */}
          <div className='flex items-center gap-2'>
            <div className='relative flex-1'>
              <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
              <Input
                placeholder={t('Search by order number...')}
                value={keyword}
                onChange={(e) => handleSearch(e.target.value)}
                className='h-9 pl-10'
              />
            </div>
            <Select
              items={[
                { value: '10', label: t('10 / page') },
                { value: '20', label: t('20 / page') },
                { value: '50', label: t('50 / page') },
                { value: '100', label: t('100 / page') },
              ]}
              value={pageSize.toString()}
              onValueChange={(value) =>
                value !== null && handlePageSizeChange(Number.parseInt(value))
              }
            >
              <SelectTrigger className='h-9 w-[92px] sm:w-32'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='10'>{t('10 / page')}</SelectItem>
                  <SelectItem value='20'>{t('20 / page')}</SelectItem>
                  <SelectItem value='50'>{t('50 / page')}</SelectItem>
                  <SelectItem value='100'>{t('100 / page')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          {/* Records List */}
          <div className='min-h-0 max-h-[min(54vh,520px)] flex-1 overflow-y-auto overscroll-contain pr-1'>
            {loading && (
              <div className='space-y-3'>
                {['first', 'second', 'third', 'fourth', 'fifth'].map(
                  (placeholder) => (
                    <div
                      key={placeholder}
                      className='rounded-lg border p-3 sm:p-4'
                    >
                      <div className='flex items-start justify-between'>
                        <div className='flex-1 space-y-2'>
                          <Skeleton className='h-4 w-48' />
                          <Skeleton className='h-3 w-32' />
                        </div>
                        <Skeleton className='h-5 w-16' />
                      </div>
                      <div className='mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4'>
                        <Skeleton className='h-3 w-full' />
                        <Skeleton className='h-3 w-full' />
                        <Skeleton className='h-3 w-full' />
                      </div>
                    </div>
                  )
                )}
              </div>
            )}
            {!loading && records.length === 0 && (
              <div className='text-muted-foreground flex min-h-40 flex-col items-center justify-center py-10 text-center'>
                <p className='text-sm font-medium'>
                  {t('No billing records found')}
                </p>
                <p className='mt-1 text-xs'>
                  {keyword
                    ? t('Try adjusting your search')
                    : t('Your transaction history will appear here')}
                </p>
              </div>
            )}
            {!loading && records.length > 0 && (
              <div className='space-y-3'>
                {records.map((record) => {
                  const statusConfig = getStatusConfig(record.status)
                  return (
                    <div
                      key={record.id}
                      className='rounded-lg border p-3 sm:p-4'
                    >
                      {/* Header Row */}
                      <div className='flex items-start justify-between gap-2'>
                        <div className='flex-1 space-y-1'>
                          <div className='flex min-w-0 items-center gap-2'>
                            <code className='text-foreground truncate font-mono text-sm'>
                              {record.trade_no}
                            </code>
                            <Button
                              variant='ghost'
                              size='sm'
                              className='h-5 w-5 p-0'
                              onClick={() => copyToClipboard(record.trade_no)}
                            >
                              {copiedText === record.trade_no ? (
                                <Check className='h-3 w-3' />
                              ) : (
                                <Copy className='h-3 w-3' />
                              )}
                            </Button>
                            {isAdmin && record.user_id != null && (
                              <StatusBadge
                                label={`${t('User ID')}: ${record.user_id}`}
                                variant='neutral'
                                size='sm'
                                copyText={String(record.user_id)}
                              />
                            )}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {formatTimestamp(record.create_time)}
                          </div>
                        </div>
                        <StatusBadge
                          label={statusConfig.label}
                          variant={statusConfig.variant}
                          showDot
                          copyable={false}
                        />
                      </div>

                      {/* Details Grid */}
                      <div className='mt-3 grid grid-cols-2 gap-3 sm:mt-4 sm:grid-cols-3 sm:gap-4'>
                        <div className='space-y-1'>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Payment Method')}
                          </Label>
                          <div className='text-sm font-medium'>
                            {getPaymentMethodName(record.payment_method, t)}
                          </div>
                        </div>
                        <div className='space-y-1'>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Amount')}
                          </Label>
                          <div className='text-sm font-semibold'>
                            {formatCurrencyFromUSD(record.amount, {
                              digitsLarge: 2,
                              digitsSmall: 2,
                              abbreviate: false,
                            })}
                          </div>
                        </div>
                        <div className='space-y-1'>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Payment')}
                          </Label>
                          <div className='text-sm font-semibold text-red-600'>
                            {formatNumber(record.money)}
                          </div>
                        </div>
                      </div>

                      {/* Admin Actions */}
                      {isAdmin && record.status === 'success' && (
                        <div className='mt-4 flex justify-end'>
                          <Button size='sm' variant='outline' onClick={() => setRefundRecord(record)}>
                            {t('Refund processing')}
                          </Button>
                        </div>
                      )}
                      {isAdmin && record.status === 'pending' && (
                        <div className='mt-4 flex justify-end'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => setConfirmTradeNo(record.trade_no)}
                            disabled={completing}
                          >
                            {t('Complete Order')}
                          </Button>
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          {/* Pagination */}
          {!loading && records.length > 0 && (
            <div className='flex flex-col items-center gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
              <div className='text-muted-foreground text-xs sm:text-sm'>
                {t('Showing')} {(page - 1) * pageSize + 1}-
                {Math.min(page * pageSize, total)} {t('of')} {total}
              </div>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(page - 1)}
                  disabled={page <= 1}
                  className='h-8 w-8 p-0'
                >
                  <ChevronLeft className='h-4 w-4' />
                </Button>
                <div className='text-muted-foreground flex items-center gap-1 text-sm'>
                  <span className='font-medium'>{page}</span>
                  <span>/</span>
                  <span>{totalPages}</span>
                </div>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(page + 1)}
                  disabled={page >= totalPages}
                  className='h-8 w-8 p-0'
                >
                  <ChevronRight className='h-4 w-4' />
                </Button>
              </div>
            </div>
          )}
        </div>
      </Dialog>

      <Dialog
        open={!!refundRecord}
        onOpenChange={(value) => !value && !refundLoading && closeRefund()}
        title={t('Refund processing')}
        description={t('Offline refund only. ZPAY is not called.')}
        contentHeight='auto'
        bodyClassName='space-y-4'
      >
        <div className='space-y-3'>
          <div className='text-muted-foreground text-sm'>{refundRecord?.trade_no}</div>
          <div className='space-y-2'>
            <Label>{refundId ? t('Manual deduction reason') : t('Review reason')}</Label>
            <Textarea value={refundReason} onChange={(event) => setRefundReason(event.target.value)} disabled={refundLoading} />
          </div>
          {refundId && (
            <>
              <div className='space-y-2'>
                <Label>{t('Deduct quota')}</Label>
                <Input type='number' min='1' step='1' value={deductQuota} onChange={(event) => setDeductQuota(event.target.value)} disabled={refundLoading} />
              </div>
              <div className='space-y-2'>
                <Label>{t('Offline payment proof reference')}</Label>
                <Input value={refundProof} onChange={(event) => setRefundProof(event.target.value)} disabled={refundLoading} />
              </div>
            </>
          )}
        </div>
        <div className='flex flex-wrap justify-end gap-2'>
          <Button variant='outline' onClick={refundId ? handleCancelRefund : closeRefund} disabled={refundLoading}>{t(refundId ? 'Cancel processing' : 'Cancel')}</Button>
          <Button onClick={refundId ? handleCompleteRefund : handleStartRefund} disabled={refundLoading || !refundReason.trim()}>{refundLoading ? t('Processing...') : t(refundId ? 'Complete refund processing' : 'Start refund processing')}</Button>
        </div>
      </Dialog>

      {/* Confirm Complete Order Dialog */}
      <AlertDialog
        open={!!confirmTradeNo}
        onOpenChange={(open) => !open && setConfirmTradeNo(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Complete Order')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to manually complete this order? The user will be credited with the corresponding quota.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={completing}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmComplete}
              disabled={completing}
            >
              {completing ? t('Processing...') : t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
