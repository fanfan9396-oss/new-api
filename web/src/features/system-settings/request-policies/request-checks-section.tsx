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
import { zodResolver } from '@hookform/resolvers/zod'
import { RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import {
  SettingsForm,
  SettingsControlGroup,
  SettingsControlChildren,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { getContentAuditModels } from './api'
import { useSavePolicy } from './use-save-policy'

const sensitiveSchema = z.object({
  CheckSensitiveEnabled: z.boolean(),
  CheckSensitiveOnPromptEnabled: z.boolean(),
  SensitiveWords: z.string().optional(),
})

const contentAuditSchema = z.object({
  ContentAuditEnabled: z.boolean(),
  ContentAuditEndpoint: z.string().max(512),
  ContentAuditModel: z.string().max(128),
  ContentAuditTimeoutMs: z.string().regex(/^\d+$/, 'Enter milliseconds'),
  ContentAuditSampleRate: z.string().regex(/^(?:0(?:\.\d+)?|1(?:\.0+)?)$/, 'Enter a value from 0 to 1'),
  ContentAuditPrompt: z.string().max(20000),
  ContentAuditFlaggedThreshold: z.string().regex(/^(?:0(?:\.\d+)?|1(?:\.0+)?)$/, 'Enter a value from 0 to 1'),
  ContentAuditReviewThreshold: z.string().regex(/^(?:0(?:\.\d+)?|1(?:\.0+)?)$/, 'Enter a value from 0 to 1'),
})

type SensitiveFormValues = z.infer<typeof sensitiveSchema>
type ContentAuditFormValues = z.infer<typeof contentAuditSchema>

type RequestChecksSectionProps = {
  defaultValues: SensitiveFormValues & ContentAuditFormValues
}

export function RequestChecksSection({
  defaultValues,
}: RequestChecksSectionProps) {
  const { t } = useTranslation()
  const updateOption = useSavePolicy()
  const form = useForm<SensitiveFormValues>({
    resolver: zodResolver(sensitiveSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: SensitiveFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof SensitiveFormValues]
    )

    try {
      if (updates.length > 0) {
        await updateOption.mutateAsync(
          Object.fromEntries(
            updates.map(([key, value]) => [key, String(value ?? '')])
          )
        )
      }
    } catch (error) {
      handleServerError(error)
    }
  }

  return (
    <>
      <SettingsSection title={t('Request checks')}>
      <p className='text-muted-foreground text-sm'>
        {t('Source: global settings. Changes take effect after saving.')}
      </p>
      <h3 className='text-sm font-medium'>{t('Request text filtering')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Checks text extracted from supported requests against keywords, ignoring case. A match rejects the request before upstream processing and does not affect channel health. Images, audio and generated responses are not checked.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={form.formState.isSubmitting}
            saveLabel='Save sensitive words'
          />
          <Alert>
            <AlertDescription>
              {form.watch('CheckSensitiveEnabled') &&
              form.watch('CheckSensitiveOnPromptEnabled') &&
              form.watch('SensitiveWords')?.trim()
                ? t(
                    'Prompt text filtering is active with the current form values.'
                  )
                : t(
                    'Prompt text filtering needs both switches enabled and a non-empty keyword list.'
                  )}
            </AlertDescription>
          </Alert>
          <SettingsControlGroup>
            <FormField
              control={form.control}
              name='CheckSensitiveEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable filtering')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Blocks messages when sensitive keywords are detected.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <SettingsControlChildren>
              <FormField
                control={form.control}
                name='CheckSensitiveOnPromptEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Inspect user prompts')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, prompts are scanned before reaching upstream models.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        disabled={!form.watch('CheckSensitiveEnabled')}
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsControlChildren>
          </SettingsControlGroup>

          <FormField
            control={form.control}
            name='SensitiveWords'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Blocked keywords')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={12}
                    placeholder={t('Enter one keyword per line')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Each line represents one keyword. Leave blank to disable the list but keep the switch states.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
      </SettingsSection>
      <ContentAuditSection defaultValues={defaultValues} />
    </>
  )
}

function ContentAuditSection({ defaultValues }: RequestChecksSectionProps) {
  const { t } = useTranslation()
  const updateOption = useSavePolicy()
  const [modelOptions, setModelOptions] = useState<string[]>([])
  const [modelFetchError, setModelFetchError] = useState('')
  const [isFetchingModels, setIsFetchingModels] = useState(false)
  const form = useForm<ContentAuditFormValues>({
    resolver: zodResolver(contentAuditSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const fetchModels = async () => {
    setIsFetchingModels(true)
    setModelFetchError('')
    try {
      const models = await getContentAuditModels(
        form.getValues('ContentAuditEndpoint')
      )
      setModelOptions(models)
      if (!form.getValues('ContentAuditModel')) {
        form.setValue('ContentAuditModel', models[0] ?? '')
      }
    } catch (error) {
      setModelFetchError(
        error instanceof Error ? error.message : t('Failed to fetch models')
      )
    } finally {
      setIsFetchingModels(false)
    }
  }

  const onSubmit = async (values: ContentAuditFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) => value !== defaultValues[key as keyof ContentAuditFormValues]
    )
    if (updates.length === 0) return
    await updateOption.mutateAsync(
      Object.fromEntries(updates.map(([key, value]) => [key, String(value)]))
    )
  }

  return (
    <SettingsSection title={t('External content audit shadow')}>
      <p className='text-muted-foreground text-sm'>
        {t('Sends sampled request text to an external auditor. Results are recorded for review only and never block, freeze, revoke, or charge.')}
      </p>
      <Alert>
        <AlertDescription>
          {t('The audit API key is never stored in New API options or shown here. Configure it in the server secret environment.')}
        </AlertDescription>
      </Alert>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={form.formState.isSubmitting}
            saveLabel='Save audit settings'
          />
          <SettingsControlGroup>
            <FormField
              control={form.control}
              name='ContentAuditEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable shadow audit')}</FormLabel>
                    <FormDescription>
                      {t('Records model review signals asynchronously without changing the request decision.')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch checked={field.value} onCheckedChange={field.onChange} />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </SettingsControlGroup>
          <Alert>
            <AlertDescription>
              {form.watch('ContentAuditEnabled')
                ? t('Shadow audit is enabled; requests continue normally and results are queued for review.')
                : t('Shadow audit is disabled; no external audit request will be sent.')}
            </AlertDescription>
          </Alert>

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ContentAuditEndpoint'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Audit endpoint')}</FormLabel>
                  <FormControl><Input placeholder='https://provider.example/v1/chat/completions' {...field} /></FormControl>
                  <FormDescription>{t('Use a direct external endpoint, not this New API gateway.')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='ContentAuditModel'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Audit model')}</FormLabel>
                  <div className='flex gap-2'>
                    <FormControl><Input list='content-audit-models' placeholder='audit-model' {...field} /></FormControl>
                    <Button type='button' variant='outline' size='sm' aria-label={t('Fetch models')} title={t('Fetch models')} disabled={isFetchingModels} onClick={() => void fetchModels()}>
                      <RefreshCw className={isFetchingModels ? 'animate-spin' : ''} />
                      <span>{t('Fetch models')}</span>
                    </Button>
                  </div>
                  <datalist id='content-audit-models'>
                    {modelOptions.map((model) => <option key={model} value={model} />)}
                  </datalist>
                  <FormDescription>{t('Use a low-cost model with reliable JSON output.')}</FormDescription>
                  {modelFetchError && <p className='text-destructive text-xs'>{modelFetchError}</p>}
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='ContentAuditTimeoutMs'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Timeout (ms)')}</FormLabel>
                  <FormControl><Input type='number' min={100} max={30000} step={100} {...field} /></FormControl>
                  <FormDescription>{t('Timed-out audits are dropped from the shadow path.')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='ContentAuditSampleRate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Sample rate')}</FormLabel>
                  <FormControl><Input type='number' min={0} max={1} step={0.01} {...field} /></FormControl>
                  <FormDescription>{t('A value from 0 to 1. Start low to control cost.')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='ContentAuditReviewThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Review threshold')}</FormLabel>
                  <FormControl><Input type='number' min={0} max={1} step={0.05} {...field} /></FormControl>
                  <FormDescription>{t('Flagged results below this confidence stay out of the review queue.')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='ContentAuditFlaggedThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Reserved blocking threshold')}</FormLabel>
                  <FormControl><Input type='number' min={0} max={1} step={0.05} {...field} /></FormControl>
                  <FormDescription>{t('Reserved for future policy review. Automatic blocking is disabled.')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='ContentAuditPrompt'
            render={({ field }) => (
              <FormItem>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <FormLabel>{t('Additional audit instructions')}</FormLabel>
                  <Button type='button' variant='outline' size='sm' onClick={() => form.setValue('ContentAuditPrompt', '')}>
                    {t('Restore default policy')}
                  </Button>
                </div>
                <FormControl><Textarea rows={10} placeholder={t('Optional additive instructions for the auditor')} {...field} /></FormControl>
                <FormDescription>{t('The immutable safety boundary remains active. These instructions cannot enable blocking or account actions.')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
