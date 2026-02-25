import useSWR, { SWRConfiguration } from "swr";

import { checkUpdateSafe, type UpdateInfo } from "@/services/update";

import { useVerge } from "./use-verge";

export type { UpdateInfo };

export const useUpdate = (
  enabled: boolean = true,
  options?: SWRConfiguration,
) => {
  const { verge } = useVerge();
  const { auto_check_update } = verge || {};

  const shouldCheck = enabled && auto_check_update !== false;

  const {
    data: updateInfo,
    mutate: checkUpdate,
    isValidating,
  } = useSWR(shouldCheck ? "checkUpdate" : null, checkUpdateSafe, {
    errorRetryCount: 2,
    revalidateIfStale: false,
    revalidateOnFocus: false,
    focusThrottleInterval: 36e5, // 1 hour
    refreshInterval: 24 * 60 * 60 * 1000, // 24 hours
    dedupingInterval: 60 * 60 * 1000, // 1 hour
    ...options,
  });

  return {
    updateInfo,
    checkUpdate,
    loading: isValidating,
  };
};
