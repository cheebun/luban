import { twc } from "react-twc";

export const Select = twc.select`
  flex h-10 w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm
  focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
  disabled:cursor-not-allowed disabled:opacity-50
`;
