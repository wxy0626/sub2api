import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { describe, it, expect } from 'vitest'

import { findRowIndexByDomPosition, useSwipeSelect } from '../useSwipeSelect'

// 用真实表头和数据单元格复现 document 级框选事件的最小测试表格。
const SwipeSelectHarness = defineComponent({
  setup() {
    const containerRef = ref<HTMLElement | null>(null)
    useSwipeSelect(containerRef, {
      isSelected: () => false,
      select: () => undefined,
      deselect: () => undefined
    })

    return () => h('div', { ref: containerRef, class: 'table-page-layout' }, [
      h('table', [
        h('thead', [
          h('tr', [
            h('th', { 'data-test': 'swipe-select-header' }, 'Header')
          ])
        ]),
        h('tbody', [
          h('tr', { 'data-row-id': '1' }, [
            h('td', { 'data-test': 'swipe-select-cell' }, 'Cell')
          ])
        ])
      ])
    ])
  }
})

// 模拟按下后浏览器准备开始文本选择，用于观察框选监听是否已注册。
function dispatchSwipeSelectStart(element: Element): Event {
  const event = new MouseEvent('mousedown', {
    bubbles: true,
    cancelable: true,
    button: 0,
    clientX: -1,
    clientY: -1
  })
  element.dispatchEvent(event)

  const selectStartEvent = new Event('selectstart', { bubbles: true, cancelable: true })
  element.dispatchEvent(selectStartEvent)
  return selectStartEvent
}

/**
 * Build a fake scroll element whose `tbody tr[data-index]` rows expose stubbed
 * vertical rects, so we can exercise findRowIndexByDomPosition without a real DOM.
 * `index` is the value placed in the row's data-index attribute (its absolute
 * position in the sorted data), which need not equal the array position.
 */
function makeScrollEl(rows: Array<{ index: number; top: number; bottom: number }>): Element {
  const trs = rows.map((r) => ({
    getAttribute: (name: string) => (name === 'data-index' ? String(r.index) : null),
    getBoundingClientRect: () => ({
      top: r.top,
      bottom: r.bottom,
      left: 0,
      right: 0,
      width: 0,
      height: r.bottom - r.top,
      x: 0,
      y: r.top,
      toJSON: () => ({})
    })
  })) as unknown as HTMLElement[]

  return {
    querySelectorAll: (sel: string) =>
      (sel === 'tbody tr[data-index]' ? trs : []) as unknown as NodeListOf<Element>
  } as unknown as Element
}

describe('findRowIndexByDomPosition (swipe-select full-render fallback)', () => {
  // Variable row heights on purpose — the third row is taller.
  const rows = [
    { index: 0, top: 100, bottom: 200 },
    { index: 1, top: 200, bottom: 300 },
    { index: 2, top: 300, bottom: 450 }
  ]
  const el = makeScrollEl(rows)

  it('returns -1 when no rows are rendered', () => {
    expect(findRowIndexByDomPosition(makeScrollEl([]), 250)).toBe(-1)
  })

  it('locates the row whose rect contains the Y coordinate', () => {
    expect(findRowIndexByDomPosition(el, 150)).toBe(0)
    expect(findRowIndexByDomPosition(el, 250)).toBe(1)
    expect(findRowIndexByDomPosition(el, 400)).toBe(2) // inside the tall row
  })

  it('clamps to the first/last row when Y is outside the rendered range', () => {
    expect(findRowIndexByDomPosition(el, 50)).toBe(0) // above the first row
    expect(findRowIndexByDomPosition(el, 999)).toBe(2) // below the last row
  })

  it('picks the closer row when Y falls in a gap between rows', () => {
    const gapped = makeScrollEl([
      { index: 0, top: 100, bottom: 180 },
      { index: 1, top: 220, bottom: 300 }
    ])
    expect(findRowIndexByDomPosition(gapped, 190)).toBe(0) // 10px from row0.bottom vs 30px from row1.top
    expect(findRowIndexByDomPosition(gapped, 215)).toBe(1) // 35px from row0.bottom vs 5px from row1.top
  })

  it('returns the data-index attribute value, not the array position', () => {
    const remapped = makeScrollEl([
      { index: 5, top: 100, bottom: 200 },
      { index: 9, top: 200, bottom: 300 }
    ])
    expect(findRowIndexByDomPosition(remapped, 150)).toBe(5)
    expect(findRowIndexByDomPosition(remapped, 250)).toBe(9)
  })
})

describe('useSwipeSelect activation boundary', () => {
  it('only starts marquee selection from a data-row cell, never from a table header', async () => {
    const wrapper = mount(SwipeSelectHarness, { attachTo: document.body })

    try {
      await nextTick()
      const header = wrapper.get('[data-test="swipe-select-header"]')
      expect(dispatchSwipeSelectStart(header.element).defaultPrevented).toBe(false)

      const dataCell = wrapper.get('[data-test="swipe-select-cell"]')
      expect(dispatchSwipeSelectStart(dataCell.element).defaultPrevented).toBe(true)
    } finally {
      document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, button: 0 }))
      wrapper.unmount()
    }
  })
})
