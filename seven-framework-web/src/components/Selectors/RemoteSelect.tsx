import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Select, Spin } from 'antd';
import type { SelectProps, DefaultOptionType } from 'antd/es/select';
import { debounce } from '@/lib/utils/debounce';

export interface RemoteSelectOption<T = unknown> {
  value: string | number;
  label: React.ReactNode;
  data?: T;
}

export interface RemoteSelectProps<
  ValueType extends string | number = number,
  OptionData = unknown,
> extends Omit<SelectProps<ValueType>, 'options' | 'optionRender'> {
  fetchOptions: (search: string) => Promise<RemoteSelectOption<OptionData>[]>;
  fetchByValue?: (value: ValueType) => Promise<RemoteSelectOption<OptionData> | null>;
  debounceTimeout?: number;
  optionRender?: (option: RemoteSelectOption<OptionData>) => React.ReactNode;
  fetchOnDropdownOpen?: boolean;
}

const toKey = (value: string | number) => `${value}`;

const RemoteSelect = <
  ValueType extends string | number = number,
  OptionData = unknown,
>({
  fetchOptions,
  fetchByValue,
  debounceTimeout = 300,
  optionRender,
  fetchOnDropdownOpen = true,
  notFoundContent,
  onDropdownVisibleChange,
  value,
  mode,
  ...restProps
}: RemoteSelectProps<ValueType, OptionData>) => {
  const [searchOptions, setSearchOptions] = useState<RemoteSelectOption<OptionData>[]>([]);
  const [globalOptions, setGlobalOptions] = useState<
    Record<string, RemoteSelectOption<OptionData>>
  >({});
  const [fetching, setFetching] = useState(false);
  const fetchIdRef = useRef(0);

  const mergeOptions = useCallback((incoming: RemoteSelectOption<OptionData>[]) => {
    if (!incoming?.length) return;

    setGlobalOptions((prev) => {
      const next = { ...prev };
      incoming.forEach((item) => {
        next[toKey(item.value)] = item;
      });
      return next;
    });
  }, []);

  const loadOptions = useCallback(
    async (keyword: string, forceImmediate = false) => {
      const triggerFetch = async () => {
        fetchIdRef.current += 1;
        const currentFetchId = fetchIdRef.current;
        setFetching(true);
        try {
          const result = await fetchOptions(keyword);
          if (fetchIdRef.current !== currentFetchId) return;
          setSearchOptions(result);
          mergeOptions(result);
        } catch (error) {
          console.error('RemoteSelect fetchOptions failed:', error);
          setSearchOptions([]);
        } finally {
          if (fetchIdRef.current === currentFetchId) {
            setFetching(false);
          }
        }
      };

      if (forceImmediate) {
        triggerFetch();
        return;
      }

      triggerFetch();
    },
    [fetchOptions, mergeOptions],
  );

  const debouncedFetcher = useMemo(
    () =>
      debounce((keyword: string) => {
        loadOptions(keyword);
      }, debounceTimeout),
    [loadOptions, debounceTimeout],
  );

  useEffect(() => () => debouncedFetcher.cancel(), [debouncedFetcher]);

  const extractValues = useCallback(
    (selectValue: SelectProps<ValueType>['value']) => {
      if (selectValue == null) {
        return [] as (string | number)[];
      }

      if (restProps.labelInValue) {
        const labeledValues = Array.isArray(selectValue) ? selectValue : [selectValue];
        return labeledValues
          .map((item) =>
            typeof item === 'object' && item !== null && 'value' in item
              ? item.value
              : undefined,
          )
          .filter((item): item is string | number =>
            typeof item === 'string' || typeof item === 'number',
          );
      }

      if (Array.isArray(selectValue)) {
        return (selectValue as (string | number)[]).filter(
          (val) => val !== undefined && val !== null,
        );
      }

      return [selectValue as string | number];
    },
    [restProps.labelInValue],
  );

  useEffect(() => {
    if (!fetchByValue) return;

    const selectedValues = extractValues(value);
    if (!selectedValues.length) return;

    const missingValues = selectedValues.filter((val) => !globalOptions[toKey(val)]);
    if (!missingValues.length) return;

    let mounted = true;
    setFetching(true);

    Promise.all(
      missingValues.map(async (val) => {
        try {
          const option = await fetchByValue(val as ValueType);
          return option ?? undefined;
        } catch (error) {
          console.error('RemoteSelect fetchByValue failed:', error);
          return undefined;
        }
      }),
    )
      .then((result) => {
        if (!mounted) return;
        const validOptions = result.filter(
          (item): item is RemoteSelectOption<OptionData> => !!item,
        );
        mergeOptions(validOptions);
      })
      .finally(() => {
        if (mounted) {
          setFetching(false);
        }
      });

    return () => {
      mounted = false;
    };
  }, [value, fetchByValue, extractValues, globalOptions, mergeOptions]);

  const optionsToRender: DefaultOptionType[] = useMemo(() => {
    const selectedValues = extractValues(value);
    const selectedOptions = selectedValues
      .map((val) => globalOptions[toKey(val)])
      .filter((opt): opt is RemoteSelectOption<OptionData> => !!opt);

    const mergedMap = new Map<string, RemoteSelectOption<OptionData>>();
    [...selectedOptions, ...searchOptions].forEach((item) => {
      mergedMap.set(toKey(item.value), item);
    });

    return Array.from(mergedMap.values()).map((item) => ({
      value: item.value,
      label: item.label,
      data: item,
    }));
  }, [extractValues, value, globalOptions, searchOptions]);

  const handleSearch = useCallback(
    (keyword: string) => {
      debouncedFetcher(keyword.trim());
    },
    [debouncedFetcher],
  );

  const handleDropdownVisibleChange = useCallback(
    (open: boolean) => {
      if (open && fetchOnDropdownOpen) {
        loadOptions('', true);
      }
      onDropdownVisibleChange?.(open);
    },
    [fetchOnDropdownOpen, loadOptions, onDropdownVisibleChange],
  );

  return (
    <Select
      showSearch
      filterOption={false}
      onSearch={handleSearch}
      value={value}
      mode={mode}
      notFoundContent={
        notFoundContent ?? (fetching ? <Spin size="small" /> : '未找到数据')
      }
      onDropdownVisibleChange={handleDropdownVisibleChange}
      options={optionsToRender}
      optionLabelProp="label"
      optionRender={
        optionRender
          ? (option) =>
              optionRender(option.data.data as RemoteSelectOption<OptionData>)
          : undefined
      }
      {...restProps}
    />
  );
};

export default RemoteSelect;
