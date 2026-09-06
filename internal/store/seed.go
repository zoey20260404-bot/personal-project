package store

// SamplePositions 示例岗位数据（表为空时自动写入，供开发演示）。
// 真实数据走官方职位表导入（国考：bm.scs.gov.cn 年度 Excel；省考：各省人事考试网）。
// 数据来源示例：2024-2025 国考/广东省考公开职位表。
func SamplePositions() []Position {
	fresh := true
	return []Position{
		{
			ExamType: "国考", Year: 2025, Province: "国家", City: "北京",
			Department: "国家税务总局北京市税务局", PositionName: "一级行政执法员（信息技术）", PositionCode: "300110001001",
			EducationReq: "本科及以上", MajorReqCategory: "计算机类、电子信息类", PoliticalReq: "不限",
			FreshGraduateReq: &fresh, WorkExperienceYearsReq: 0,
			Remarks: "需 24 小时值班", Score2025: 138, Score2024: 135, ApplicantRatio2025: "1:80",
		},
		{
			ExamType: "国考", Year: 2025, Province: "国家", City: "广州",
			Department: "国家税务总局广东省税务局", PositionName: "一级行政执法员", PositionCode: "300110002003",
			EducationReq: "本科及以上", MajorReqCategory: "计算机类", PoliticalReq: "中共党员或共青团员",
			FreshGraduateReq: &fresh, WorkExperienceYearsReq: 0,
			Score2025: 142, Score2024: 138, ApplicantRatio2025: "1:95",
		},
		{
			ExamType: "省考", Year: 2025, Province: "广东", City: "广州",
			Department: "广州市市场监督管理局", PositionName: "信息化管理岗", PositionCode: "GD2025A012",
			EducationReq: "本科", MajorReqCategory: "计算机类", PoliticalReq: "不限",
			WorkExperienceYearsReq: 2,
			Remarks:                "服务期 5 年", Score2025: 128, Score2024: 125, ApplicantRatio2025: "1:35",
		},
		{
			ExamType: "省考", Year: 2025, Province: "广东", City: "深圳",
			Department: "深圳市发展和改革委员会", PositionName: "综合管理岗", PositionCode: "GD2025B034",
			EducationReq: "硕士", MajorReqCategory: "经济学类", PoliticalReq: "不限",
			Score2025: 133, Score2024: 130, ApplicantRatio2025: "1:60",
		},
		{
			ExamType: "省考", Year: 2025, Province: "广东", City: "汕头",
			Department: "汕头市龙湖区人民政府办公室", PositionName: "综合文字岗", PositionCode: "GD2025C088",
			EducationReq: "本科", MajorReqCategory: "不限", PoliticalReq: "不限",
			WorkExperienceYearsReq: 0,
			Score2025:              118, Score2024: 115, ApplicantRatio2025: "1:25",
		},
		{
			ExamType: "省考", Year: 2025, Province: "广东", City: "韶关",
			Department: "韶关市曲江区农业农村局", PositionName: "农业技术推广岗", PositionCode: "GD2025D121",
			EducationReq: "大专", MajorReqCategory: "不限", PoliticalReq: "不限",
			WorkExperienceYearsReq: 0,
			Remarks:                "偏远地区，最低服务年限 5 年", Score2025: 105, Score2024: 102, ApplicantRatio2025: "1:12",
		},
	}
}
