import { useQuery } from "@tanstack/react-query";
import { GetStyleSample } from "@/lib/wailsjs/go/app/App";
import { styleSampleKeys } from "@/lib/queryKeys";

// useStyleSample: 风格样本详情 query（StyleView 编辑弹窗按需拉取单条完整 content）。
// ListStyleSamples 返回的 items.content 是 preview 截断，编辑需完整 content，故 detail 独立 fetch。
// enabled: id != null（detailId 为 null 时不 fetch）；queryKey 含 id。
// 调用方（StyleView openDetail）改 setDetailId 触发 query，useEffect 监听 query.data ready 回填编辑字段。
// update mutation onSuccess 失效 detail（commit 2）。
// 5.3 commit 1：GET 错误由全局中间件接管，组件不挂 toastError。
export function useStyleSample(id: number | null) {
  return useQuery({
    queryKey: styleSampleKeys.detail(id ?? 0),
    queryFn: async () => {
      return await GetStyleSample(id as number);
    },
    enabled: id != null,
  });
}
