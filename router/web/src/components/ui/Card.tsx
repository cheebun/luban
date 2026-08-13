import { twc } from "react-twc";

export const Card = twc.div`
  rounded-lg border border-gray-200 bg-white shadow-sm
`;

export const CardHeader = twc.div`
  flex flex-col gap-1 border-b border-gray-100 px-6 py-4
`;

export const CardTitle = twc.h3`
  text-base font-semibold text-gray-900
`;

export const CardDescription = twc.p`
  text-sm text-gray-500
`;

export const CardBody = twc.div`
  px-6 py-4
`;

export const CardFooter = twc.div`
  flex items-center gap-3 border-t border-gray-100 px-6 py-4
`;
