import { describe, it, expect } from 'vitest'
import { emptySegment, filterSegmentsBySeq, rebuildTurns, type TurnSegment } from './types'
import type { session } from '@/lib/wailsjs/go/models'

describe('emptySegment', () => {
  it('defaults firstSeq to 0 (historical, never cleared)', () => {
    const seg = emptySegment('seg_1')
    expect(seg.firstSeq).toBe(0)
  })
})

describe('rebuildTurns', () => {
  it('creates segments with firstSeq=0 (historical, never cleared)', () => {
    const messages = [
      { role: 'user', content: 'hello', turn_id: 1, event_type: 'message', agent_type: 'main' } as unknown as session.Message,
      { role: 'assistant', content: 'hi', turn_id: 1, event_type: 'message', agent_type: 'main' } as unknown as session.Message,
    ]
    const turns = rebuildTurns(messages)
    expect(turns).toHaveLength(1)
    expect(turns[0].segments.length).toBeGreaterThan(0)
    for (const seg of turns[0].segments) {
      expect(seg.firstSeq).toBe(0)
    }
  })
})

describe('filterSegmentsBySeq', () => {
  const mkSeg = (id: string, firstSeq: number, extra: Partial<TurnSegment> = {}): TurnSegment => ({
    ...emptySegment(id),
    firstSeq,
    ...extra,
  })

  it('clears current streamLoop segments (single retry, thinking interrupted)', () => {
    const segments = [
      mkSeg('hist', 0),   // 历史
      mkSeg('cur1', 1),   // 本轮第一个
      mkSeg('cur2', 2),   // 本轮第二个
    ]
    // 后端 streamStartSeq = *eventSeq + 1 = 1，本轮第一个事件 seq=1
    const result = filterSegmentsBySeq(segments, 1)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('hist')
  })

  it('clears thinking-done + content-interrupted segments (P2 bug scenario)', () => {
    // thinking 已 done（isStreaming=false）+ content 流式中断
    const segments = [
      mkSeg('thinking', 5, { thinkingDone: true, isStreaming: false, thinkingContent: '...' }),
      mkSeg('content', 6, { isStreaming: true, content: 'partial' }),
    ]
    // clear_from_seq = 5（本轮起点），firstSeq 5 和 6 都 >= 5，都应被清空
    const result = filterSegmentsBySeq(segments, 5)
    expect(result).toHaveLength(0)
  })

  it('preserves all 10 historical rounds when 11th round retries', () => {
    // 10 轮 tool 调用，每轮一个 segment
    const segments = Array.from({ length: 10 }, (_, i) => mkSeg(`r${i + 1}`, i + 1))
    // 第 11 轮起点 seq=11，clear_from_seq=11
    const result = filterSegmentsBySeq(segments, 11)
    expect(result).toHaveLength(10) // 前 10 轮全保留
    expect(result.map(s => s.id)).toEqual(['r1', 'r2', 'r3', 'r4', 'r5', 'r6', 'r7', 'r8', 'r9', 'r10'])
  })

  it('consecutive retries clear each round independently', () => {
    // 第一轮 segments
    const round1 = [mkSeg('r1_1', 1), mkSeg('r1_2', 2)]
    // 第一轮重试：clear_from_seq=1
    const afterFirstRetry = filterSegmentsBySeq(round1, 1)
    expect(afterFirstRetry).toHaveLength(0)

    // 第二轮 segments（seq 从 5 开始）
    const round2 = [...afterFirstRetry, mkSeg('r2_1', 5), mkSeg('r2_2', 6)]
    // 第二轮重试：clear_from_seq=5
    const afterSecondRetry = filterSegmentsBySeq(round2, 5)
    expect(afterSecondRetry).toHaveLength(0)
  })

  it('preserves historical segments (firstSeq=0) when clearFromSeq=1', () => {
    const segments = [
      mkSeg('hist', 0),  // rebuildTurns 创建的历史 segment
      mkSeg('cur', 1),   // 本轮
    ]
    const result = filterSegmentsBySeq(segments, 1)
    expect(result).toHaveLength(1)
    expect(result[0].id).toBe('hist')
  })

  it('handles empty segments array', () => {
    const result = filterSegmentsBySeq([], 1)
    expect(result).toHaveLength(0)
  })

  it('clears everything when clearFromSeq=0 (should not happen in practice)', () => {
    // 边界场景：如果后端 bug 导致 clear_from_seq=0，会清空所有 segments
    // 这正是为什么后端必须用 *eventSeq + 1 而不是 *eventSeq
    const segments = [
      mkSeg('hist', 0),
      mkSeg('cur', 1),
    ]
    const result = filterSegmentsBySeq(segments, 0)
    expect(result).toHaveLength(0)
  })

  it('does not clear segments from previous round with same firstSeq', () => {
    // 多轮场景：前一轮 segments 保留，本轮清空
    const segments = [
      mkSeg('prev1', 1),   // 上一轮
      mkSeg('prev2', 2),   // 上一轮
      mkSeg('cur1', 5),    // 本轮
      mkSeg('cur2', 6),    // 本轮
    ]
    // 本轮起点 seq=5
    const result = filterSegmentsBySeq(segments, 5)
    expect(result).toHaveLength(2)
    expect(result.map(s => s.id)).toEqual(['prev1', 'prev2'])
  })
})
