import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('../client', () => ({
  apiClient: {
    get,
    post,
  },
}))

import {
  deployLatestPersonalVersion,
  getPersonalDeploymentVersions,
  rollback,
  type PersonalDeploymentVersion
} from '@/api/admin/system'

describe('admin system rollback API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('getPersonalDeploymentVersions fetches the verified personal image list', async () => {
    const versions: PersonalDeploymentVersion[] = [
      {
        tag: 'v0.1.162-custom.1',
        commit: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        digest: 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
        reference: 'ghcr.io/example/sub2api@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
      }
    ]
    get.mockResolvedValue({ data: { versions } })

    const result = await getPersonalDeploymentVersions()

    expect(get).toHaveBeenCalledWith('/admin/system/deployment-versions')
    expect(result.versions).toEqual(versions)
  })

  it('deployLatestPersonalVersion invokes the distinct personal update endpoint', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: false, restart_scheduled: true } })

    await deployLatestPersonalVersion()

    expect(post).toHaveBeenCalledWith(
      '/admin/system/deployment/update',
      undefined,
      { timeout: 15 * 60 * 1000 }
    )
  })

  it('rollback posts the allowlisted personal tag and digest in the request body', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true } })
    const tag = 'v0.1.162-custom.1'
    const digest = 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'

    const result = await rollback(tag, digest)

    expect(post).toHaveBeenCalledWith(
      '/admin/system/rollback',
	  { tag, digest },
      { timeout: 15 * 60 * 1000 }
    )
    expect(result.need_restart).toBe(true)
  })
})
