import { describe, expect, test } from 'vitest'

import { getStatusConfig } from './billing'

describe('billing status presentation', () => {
  test('keeps failed payment orders visibly failed', () => {
    expect(getStatusConfig('failed')).toEqual({
      variant: 'danger',
      label: 'Failed',
    })
  })
})
