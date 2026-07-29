import { createContext, useContext } from "react";

// 侧边栏 List 与主区 View 是兄弟组件，各自独立 fetch 数据。
// View 保存/删除后只刷自己的 state，侧边栏 List 不会重载（计数陈旧）。
// 这里用一个轻量 Context 传"刷新信号"：View bump → nonce 变 → List 的 useEffect 重载。
// 默认值 no-op，保证 List/View 在测试或脱离 Provider 渲染时不报错。
interface RefreshState {
  refreshNonce: number;
  bumpRefresh: () => void;
}

const defaultState: RefreshState = {
  refreshNonce: 0,
  bumpRefresh: () => {},
};

export const RefreshContext = createContext<RefreshState>(defaultState);

// useRefresh 由 List（消费 refreshNonce）和 View（消费 bumpRefresh）调用。
export function useRefresh(): RefreshState {
  return useContext(RefreshContext);
}
