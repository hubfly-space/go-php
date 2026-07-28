import { useState, useEffect, useCallback, useRef } from 'react'

export function useApi<T>(fetcher: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const mountedRef = useRef(true)

  const load = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await fetcher()
      if (mountedRef.current) setData(result)
    } catch (e) {
      if (mountedRef.current) setError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  }, deps)

  useEffect(() => {
    mountedRef.current = true
    load()
    return () => { mountedRef.current = false }
  }, [load])

  return { data, loading, error, refetch: load }
}

export function useInterval(fn: () => void, ms: number | null) {
  const savedFn = useRef(fn)
  useEffect(() => { savedFn.current = fn }, [fn])
  useEffect(() => {
    if (ms === null) return
    const id = setInterval(() => savedFn.current(), ms)
    return () => clearInterval(id)
  }, [ms])
}
